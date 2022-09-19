package operator

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	k8sv1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers"
	"go.mondoo.com/mondoo-operator/controllers/integration"
	"go.mondoo.com/mondoo-operator/controllers/resource_monitor"
	"go.mondoo.com/mondoo-operator/controllers/resource_monitor/scan_api_store"
	"go.mondoo.com/mondoo-operator/controllers/status"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	//+kubebuilder:scaffold:imports
)

var Cmd = &cobra.Command{
	Use:   "operator",
	Short: "Starts the Mondoo Operator",
}

func init() {
	metricsAddr := Cmd.Flags().String("metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	probeAddr := Cmd.Flags().String("health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	enableLeaderElection := Cmd.Flags().Bool("leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		// TODO: opts.BindFlags(flag.CommandLine) is not supported with cobra. If we want to support that we should manually
		// implement reading the flags.
		ctrl.SetLogger(logger.NewLogger())
		setupLog := ctrl.Log.WithName("setup")

		scheme := runtime.NewScheme()

		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		utilruntime.Must(k8sv1alpha2.AddToScheme(scheme))
		//+kubebuilder:scaffold:scheme
		utilruntime.Must(certmanagerv1.AddToScheme(scheme))
		utilruntime.Must(monitoringv1.AddToScheme(scheme))

		mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
			Scheme:                 scheme,
			MetricsBindAddress:     *metricsAddr,
			Port:                   9443,
			HealthProbeBindAddress: *probeAddr,
			LeaderElection:         *enableLeaderElection,
			LeaderElectionID:       "60679458.mondoo.com",
			ClientDisableCacheFor: []client.Object{
				// Don't cache so we can do a Get() on a Secret without a background List()
				// trying to cache things we don't have access to
				&corev1.Secret{},
			},
		})
		if err != nil {
			setupLog.Error(err, "unable to start manager")
			return err
		}

		v, err := k8s.GetServerVersion(mgr.GetConfig())
		if err != nil {
			setupLog.Error(err, "failed to retrieve server version")
			return err
		}

		passed, err := preflightApiChecks(setupLog)
		if err != nil {
			setupLog.Error(err, "error while performing preflight validation")
			return err
		}
		if !passed {
			msg := "required API(s) not found"
			setupLog.Info(msg)
			return fmt.Errorf(msg)
		}

		// The API group "config.openshift.io" should be unique to an OpenShift cluster
		isOpenShift, err := k8s.VerifyAPI("config.openshift.io", "v1", setupLog)
		if err != nil {
			setupLog.Error(err, "error while checking if running on OpenShift")
			return err
		}

		ctx := ctrl.SetupSignalHandler()

		scanApiStore := scan_api_store.NewScanApiStore(ctx)
		go scanApiStore.Start()
		if err := preloadScanApiUrls(scanApiStore, scheme, setupLog); err != nil {
			setupLog.Error(err, "failed to preload scan API URLs")
			return err
		}
		if err = (&controllers.MondooAuditConfigReconciler{
			Client:                 mgr.GetClient(),
			MondooClientBuilder:    controllers.MondooClientBuilder,
			ContainerImageResolver: mondoo.NewContainerImageResolver(isOpenShift),
			StatusReporter:         status.NewStatusReporter(mgr.GetClient(), controllers.MondooClientBuilder, v),
			RunningOnOpenShift:     isOpenShift,
			ScanApiStore:           scanApiStore,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MondooAuditConfig")
			return err
		}
		if err = (&controllers.MondooOperatorConfigReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "MondooOperatorConfig")
			return err
		}

		if err = resource_monitor.RegisterResourceMonitors(ctx, mgr, scanApiStore); err != nil {
			setupLog.Error(err, "unable to register resource monitors", "controller", "resource_monitor")
			return err
		}

		if err = integration.Add(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Integration")
			return err
		}

		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up health check")
			return err
		}

		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up ready check")
			return err
		}

		setupLog.Info("starting manager", "operator-version", version.Version, "k8s-version", v.GitVersion)
		if err := mgr.Start(ctx); err != nil {
			setupLog.Error(err, "problem running manager")
			return err
		}

		return nil
	}
}

func preflightApiChecks(log logr.Logger) (bool, error) {
	gvrs := []struct{ g, v, r string }{
		{
			g: batchv1.SchemeGroupVersion.Group,
			v: batchv1.SchemeGroupVersion.Version,
			r: "jobs",
		},
		{
			g: batchv1.SchemeGroupVersion.Group,
			v: batchv1.SchemeGroupVersion.Version,
			r: "cronjobs",
		},
	}

	for _, gvr := range gvrs {
		exists, err := k8s.VerifyResourceExists(gvr.g, gvr.v, gvr.r, log)
		if err != nil {
			return false, err
		}
		if !exists {
			log.Error(fmt.Errorf("%s.%s.%s not found", gvr.r, gvr.v, gvr.g), "missing required resource. Mondoo Operator requires features from Kubernetes 1.21 (or later)")
			return exists, nil
		}
	}

	return true, nil
}

func preloadScanApiUrls(scanApiStore scan_api_store.ScanApiStore, scheme *runtime.Scheme, log logr.Logger) error {
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "unable to get k8s config")
		return err
	}

	kubeClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "unable to create k8s client")
		return err
	}

	ctx := context.Background()
	auditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := kubeClient.List(ctx, auditConfigs); err != nil {
		return err
	}

	for _, auditConfig := range auditConfigs.Items {
		if err := scan_api_store.HandleAuditConfig(ctx, kubeClient, scanApiStore, auditConfig); err != nil {
			log.Error(err, "failed to handle audit config", "namespace", auditConfig.Namespace, "name", auditConfig.Name)
			return err
		}
	}
	return nil
}
