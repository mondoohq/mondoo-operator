package operator

import (
	"github.com/spf13/cobra"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	certmanagerv1 "github.com/jetstack/cert-manager/pkg/apis/certmanager/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	k8sv1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers"
	"go.mondoo.com/mondoo-operator/controllers/integration"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"go.mondoo.com/mondoo-operator/pkg/version"
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
		opts := zap.Options{
			Development: true,
		}
		ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
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

		if err = (&controllers.MondooAuditConfigReconciler{
			Client:                 mgr.GetClient(),
			Scheme:                 mgr.GetScheme(),
			MondooClientBuilder:    controllers.MondooClientBuilder,
			ContainerImageResolver: mondoo.NewContainerImageResolver(),
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

		if err = integration.Add(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Integration")
			return err
		}

		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up health check")
			return err
		}

		// TODO: add a ready check that verifies whether all CRD statuses state they are upgraded to
		// the current version of the operator
		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			setupLog.Error(err, "unable to set up ready check")
			return err
		}

		setupLog.Info("starting manager", "version", version.Version)
		if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
			setupLog.Error(err, "problem running manager")
			return err
		}

		return nil
	}
	// TODO: set RunE
}
