// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package webhook

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"go.mondoo.com/mondoo-operator/pkg/version"
	webhookhandler "go.mondoo.com/mondoo-operator/pkg/webhooks/handler"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var Cmd = &cobra.Command{
	Use:   "webhook",
	Short: "Starts the Mondoo Validating Webhook",
}

func init() {
	scanApiUrl := Cmd.Flags().String("scan-api-url", "", "The URL of the service to send scan requests to.")
	tokenFilePath := Cmd.Flags().String("token-file-path", "", "Path to a file containing token to use when making scan requests.")
	webhookMode := Cmd.Flags().String("enforcement-mode", string(v1alpha2.Permissive), "Mode 'permissive' allows resources that had a failing scan result pass, and mode 'enforcing' will deny resources with failed scanning result.")
	integrationMRN := Cmd.Flags().String("integration-mrn", "", "The Mondoo integration MRN to label scanned items with if the MondooAuditConfig is configured with Mondoo integration.")
	clusterID := Cmd.Flags().String("cluster-id", "", "A cluster-unique ID for associating the webhook payloads with the underlying cluster.")
	includeNamespaces := Cmd.Flags().StringSlice("namespaces", nil, "Only process k8s resources matching the provided list of Namespaces.")
	excludeNamespaces := Cmd.Flags().StringSlice("namespaces-exclude", nil, "Ignore k8s resources matching the provided list of Namespaces.")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		webhookLog := log.Log.WithName("webhook")

		if *scanApiUrl == "" {
			return fmt.Errorf("--scan-api-url must be provided")
		}
		if *tokenFilePath == "" {
			return fmt.Errorf("--token-file-path must be provided")
		}
		if *clusterID == "" {
			return fmt.Errorf("--cluster-id must be provided")
		}

		tokenBytes, err := os.ReadFile(*tokenFilePath)
		if err != nil {
			webhookLog.Error(err, "Failed to read in file with token content")
			return err
		}
		token := strings.TrimSuffix(string(tokenBytes), "\n")

		// Setup a Manager
		webhookLog.Info("setting up manager")
		mgr, err := manager.New(config.GetConfigOrDie(), manager.Options{
			HealthProbeBindAddress: ":8081",
		})
		if err != nil {
			webhookLog.Error(err, "unable to set up overall controller manager")
			return err
		}

		// Setup webhooks
		webhookLog.Info("setting up webhook server", "version", version.Version, "commit", version.Commit)
		hookServer := mgr.GetWebhookServer()

		webhookLog.Info("registering webhooks to the webhook server")

		webhookOpts := &webhookhandler.NewWebhookValidatorOpts{
			Client:            mgr.GetClient(),
			Mode:              *webhookMode,
			ScanUrl:           *scanApiUrl,
			Token:             token,
			IntegrationMrn:    *integrationMRN,
			ClusterId:         *clusterID,
			IncludeNamespaces: *includeNamespaces,
			ExcludeNamespaces: *excludeNamespaces,
		}
		webhookValidator, err := webhookhandler.NewWebhookValidator(webhookOpts)
		if err != nil {
			webhookLog.Error(err, "failed to setup Core Webhook")
			return err
		}
		hookServer.Register("/validate-k8s-mondoo-com", &webhook.Admission{Handler: webhookValidator})

		if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
			webhookLog.Error(err, "unable to set up health check")
			return err
		}
		if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
			webhookLog.Error(err, "unable to set up ready check")
			return err
		}

		webhookLog.Info("starting manager")
		if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
			webhookLog.Error(err, "unable to run manager")
			return err
		}

		return nil
	}
}
