// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package cleanup

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"go.mondoo.com/mondoo-operator/api/v1alpha2"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha2.AddToScheme(scheme))
}

// Cmd is the cleanup subcommand
var Cmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleans up MondooAuditConfig resources before operator uninstallation",
	Long: `This command deletes all MondooAuditConfig resources in the specified namespace,
allowing finalizers to clean up operator-created resources (CronJobs, Deployments, etc.)
before the operator and CRDs are removed.

This is typically run as a Helm pre-delete hook.`,
}

func init() {
	namespace := Cmd.Flags().String("namespace", "", "The namespace to clean up MondooAuditConfig resources from (required)")
	timeout := Cmd.Flags().Duration("timeout", 2*time.Minute, "Timeout for waiting for resources to be deleted")
	pollInterval := Cmd.Flags().Duration("poll-interval", 2*time.Second, "Interval between polling for resource deletion")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		lgr := log.Log.WithName("cleanup")

		if *namespace == "" {
			return fmt.Errorf("--namespace is required")
		}

		// Set up signal handling
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigChan
			lgr.Info("Received shutdown signal", "signal", sig)
			cancel()
		}()

		// Get Kubernetes config
		restConfig, err := config.GetConfig()
		if err != nil {
			return fmt.Errorf("failed to get Kubernetes config: %w", err)
		}

		return CleanupCmd(ctx, restConfig, *namespace, *pollInterval, lgr)
	}
}

// CleanupCmd performs the cleanup of MondooAuditConfig resources
func CleanupCmd(ctx context.Context, restConfig *rest.Config, namespace string, pollInterval time.Duration, lgr logr.Logger) error {
	// Create a Kubernetes client
	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	lgr.Info("Starting cleanup of MondooAuditConfig resources", "namespace", namespace)

	// List all MondooAuditConfigs in the namespace
	auditConfigs := &v1alpha2.MondooAuditConfigList{}
	if err := k8sClient.List(ctx, auditConfigs, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("failed to list MondooAuditConfigs: %w", err)
	}

	if len(auditConfigs.Items) == 0 {
		lgr.Info("No MondooAuditConfig resources found", "namespace", namespace)
		return nil
	}

	lgr.Info("Found MondooAuditConfig resources to delete", "count", len(auditConfigs.Items))

	// Delete each MondooAuditConfig
	for i := range auditConfigs.Items {
		ac := &auditConfigs.Items[i]
		lgr.Info("Deleting MondooAuditConfig", "name", ac.Name, "namespace", ac.Namespace)
		if err := k8sClient.Delete(ctx, ac); err != nil {
			lgr.Error(err, "Failed to delete MondooAuditConfig", "name", ac.Name)
			// Continue trying to delete others
		}
	}

	// Wait for all MondooAuditConfigs to be deleted (finalizers to complete)
	lgr.Info("Waiting for MondooAuditConfig resources to be fully deleted...")
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			lgr.Info("Timeout or cancellation while waiting for cleanup to complete")
			// Check one more time with a fresh context
			checkCtx, checkCancel := context.WithTimeout(context.Background(), 5*time.Second)
			remaining := &v1alpha2.MondooAuditConfigList{}
			if err := k8sClient.List(checkCtx, remaining, client.InNamespace(namespace)); err == nil && len(remaining.Items) > 0 {
				lgr.Info("Warning: Some MondooAuditConfig resources may not have been fully cleaned up", "remaining", len(remaining.Items))
			}
			checkCancel()
			return nil
		case <-ticker.C:
			remaining := &v1alpha2.MondooAuditConfigList{}
			if err := k8sClient.List(ctx, remaining, client.InNamespace(namespace)); err != nil {
				lgr.Error(err, "Failed to list remaining MondooAuditConfigs")
				continue
			}
			if len(remaining.Items) == 0 {
				lgr.Info("All MondooAuditConfig resources cleaned up successfully")
				return nil
			}
			lgr.Info("Waiting for MondooAuditConfig resources to be deleted", "remaining", len(remaining.Items))
		}
	}
}
