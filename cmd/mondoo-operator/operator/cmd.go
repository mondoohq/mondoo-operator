// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package operator

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/utils/ptr"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	k8sv1alpha2 "go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/controllers"
	"go.mondoo.com/mondoo-operator/controllers/integration"
	"go.mondoo.com/mondoo-operator/controllers/metrics"
	"go.mondoo.com/mondoo-operator/controllers/status"
	"go.mondoo.com/mondoo-operator/pkg/utils/k8s"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
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
			Metrics:                metricsserver.Options{BindAddress: *metricsAddr},
			HealthProbeBindAddress: *probeAddr,
			LeaderElection:         *enableLeaderElection,
			LeaderElectionID:       "60679458.mondoo.com",
			LeaseDuration:          ptr.To(30 * time.Second),
			RenewDeadline:          ptr.To(20 * time.Second),
			Client: client.Options{
				Cache: &client.CacheOptions{
					DisableFor: []client.Object{
						// Don't cache so we can do a Get() on a Secret without a background List()
						// trying to cache things we don't have access to
						&corev1.Secret{},
					},
				},
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
			return errors.New(msg)
		}

		isOpenShift, err := k8s.IsOpenshift()
		if err != nil {
			setupLog.Error(err, "error while checking if running on OpenShift")
			return err
		}

		ctx := ctrl.SetupSignalHandler()

		containerImageResolver := mondoo.NewContainerImageResolver(mgr.GetClient(), isOpenShift)
		if err = (&controllers.MondooAuditConfigReconciler{
			Client:                 mgr.GetClient(),
			MondooClientBuilder:    controllers.MondooClientBuilder,
			ContainerImageResolver: containerImageResolver,
			StatusReporter:         status.NewStatusReporter(mgr.GetClient(), controllers.MondooClientBuilder, v, containerImageResolver),
			RunningOnOpenShift:     isOpenShift,
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

		// Check whether the mondoo-operator crashed because of OOMKilled
		setupLog.Info("Checking whether mondoo-operator was terminated before")

		k8sConfig, err := ctrl.GetConfig()
		if err != nil {
			setupLog.Error(err, "unable to get k8s config")
			return err
		}
		// use separate client to prevent errors due to cache
		// "the cache is not started, can not read objects"
		// https://sdk.operatorframework.io/docs/building-operators/golang/references/client/#non-default-client
		client, err := client.New(k8sConfig, client.Options{Scheme: scheme})
		if err != nil {
			setupLog.Error(err, "unable to create non-caching k8s client")
			return err
		}
		err = checkForTerminatedState(ctx, client, v, isOpenShift, setupLog)
		if err != nil {
			setupLog.Error(err, "unable to check for terminated state of mondoo-operator-controller")
		}

		if err = integration.Add(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Integration")
			return err
		}

		if err = metrics.Add(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Metrics")
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

		setupLog.Info("starting manager", "operator-version", version.Version, "operator-commit", version.Commit, "k8s-version", v.GitVersion)
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
