// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package resource_watcher

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"go.mondoo.com/mondoo-operator/controllers/resource_watcher"
	annot "go.mondoo.com/mondoo-operator/pkg/annotations"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

// Cmd is the cobra command for the resource-watcher subcommand.
var Cmd = &cobra.Command{
	Use:   "resource-watcher",
	Short: "Watches K8s resources and scans changes in real-time",
	Long: `The resource-watcher command starts a long-running process that watches
Kubernetes resources for changes (creates, updates) and scans them using cnspec.

Changes are batched using a debounce interval to prevent excessive scanning when
multiple resources change in quick succession.

Example:
  mondoo-operator resource-watcher --config /etc/mondoo/mondoo.yml --debounce-interval 10s`,
}

func init() {
	configPath := Cmd.Flags().String("config", "", "The path to the mondoo.yml config file containing service account credentials. (required)")
	namespaces := Cmd.Flags().StringSlice("namespaces", nil, "Namespaces to watch (comma-separated). Empty means all namespaces.")
	namespacesExclude := Cmd.Flags().StringSlice("namespaces-exclude", nil, "Namespaces to exclude from watching (comma-separated).")
	debounceInterval := Cmd.Flags().Duration("debounce-interval", 10*time.Second, "How long to batch changes before scanning.")
	minimumScanInterval := Cmd.Flags().Duration("minimum-scan-interval", 2*time.Minute, "Minimum time between scans (rate limit).")
	watchAllResources := Cmd.Flags().Bool("watch-all-resources", false, "Watch all resource types including ephemeral ones (Pods, Jobs). Default is to only watch high-priority resources (Deployments, DaemonSets, StatefulSets, ReplicaSets).")
	resourceTypes := Cmd.Flags().StringSlice("resource-types", nil, "Resource types to watch (comma-separated). Overrides --watch-all-resources if specified.")
	apiProxy := Cmd.Flags().String("api-proxy", "", "HTTP proxy to use for API requests.")
	timeout := Cmd.Flags().Duration("timeout", 25*time.Minute, "Timeout for scan operations.")
	annotations := Cmd.Flags().StringToString("annotation", nil, "Annotations to add to scanned assets (can specify multiple, e.g., --annotation env=prod --annotation team=platform).")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		logger := log.Log.WithName("resource-watcher")

		// Validate required flags
		if *configPath == "" {
			return fmt.Errorf("--config must be provided")
		}

		// Check if config file exists
		if _, err := os.Stat(*configPath); os.IsNotExist(err) {
			return fmt.Errorf("config file does not exist: %s", *configPath)
		}

		// Parse resource types
		var resourceTypesList []string
		if len(*resourceTypes) > 0 {
			for _, rt := range *resourceTypes {
				// Handle comma-separated values within a single flag value
				for _, t := range strings.Split(rt, ",") {
					t = strings.TrimSpace(t)
					if t != "" {
						resourceTypesList = append(resourceTypesList, t)
					}
				}
			}
		}

		// Parse namespaces
		var namespacesList []string
		if len(*namespaces) > 0 {
			for _, ns := range *namespaces {
				for _, n := range strings.Split(ns, ",") {
					n = strings.TrimSpace(n)
					if n != "" {
						namespacesList = append(namespacesList, n)
					}
				}
			}
		}

		// Parse namespaces exclude
		var namespacesExcludeList []string
		if len(*namespacesExclude) > 0 {
			for _, ns := range *namespacesExclude {
				for _, n := range strings.Split(ns, ",") {
					n = strings.TrimSpace(n)
					if n != "" {
						namespacesExcludeList = append(namespacesExcludeList, n)
					}
				}
			}
		}

		// Validate annotations
		if err := annot.Validate(*annotations); err != nil {
			return fmt.Errorf("invalid annotations: %w", err)
		}

		logger.Info("Starting resource watcher",
			"config", *configPath,
			"namespaces", namespacesList,
			"namespacesExclude", namespacesExcludeList,
			"debounceInterval", *debounceInterval,
			"minimumScanInterval", *minimumScanInterval,
			"watchAllResources", *watchAllResources,
			"resourceTypes", resourceTypesList,
			"timeout", *timeout,
			"annotations", *annotations)

		// Create context with signal handling
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Handle shutdown signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigChan
			logger.Info("Received shutdown signal", "signal", sig)
			cancel()
		}()

		// Get Kubernetes config
		restConfig, err := config.GetConfig()
		if err != nil {
			return fmt.Errorf("failed to get Kubernetes config: %w", err)
		}

		// Create cache
		cacheOpts := cache.Options{
			Scheme: scheme,
		}

		// If specific namespaces are provided, configure cache to only watch those
		if len(namespacesList) > 0 {
			byNamespace := make(map[string]cache.Config)
			for _, ns := range namespacesList {
				byNamespace[ns] = cache.Config{}
			}
			cacheOpts.DefaultNamespaces = byNamespace
		}

		c, err := cache.New(restConfig, cacheOpts)
		if err != nil {
			return fmt.Errorf("failed to create cache: %w", err)
		}

		// Create scanner
		scanner := resource_watcher.NewScanner(resource_watcher.ScannerConfig{
			ConfigPath:  *configPath,
			APIProxy:    *apiProxy,
			Timeout:     *timeout,
			Annotations: *annotations,
		})

		// Create debouncer with rate limiting
		debouncer := resource_watcher.NewDebouncer(*debounceInterval, *minimumScanInterval, scanner.ScanManifestsFunc())

		// Create watcher
		watcher := resource_watcher.NewResourceWatcher(c, debouncer, resource_watcher.WatcherConfig{
			Namespaces:        namespacesList,
			NamespacesExclude: namespacesExcludeList,
			ResourceTypes:     resourceTypesList,
			WatchAllResources: *watchAllResources,
		}, scheme)

		// Start components
		errChan := make(chan error, 3)

		// Start cache
		go func() {
			logger.Info("Starting cache")
			if err := c.Start(ctx); err != nil {
				errChan <- fmt.Errorf("cache failed: %w", err)
			}
		}()

		// Wait for cache sync
		logger.Info("Waiting for cache sync")
		if !c.WaitForCacheSync(ctx) {
			return fmt.Errorf("failed to sync cache")
		}
		logger.Info("Cache synced")

		// Start debouncer
		go func() {
			if err := debouncer.Start(ctx); err != nil {
				errChan <- fmt.Errorf("debouncer failed: %w", err)
			}
		}()

		// Start watcher
		go func() {
			if err := watcher.Start(ctx); err != nil {
				errChan <- fmt.Errorf("watcher failed: %w", err)
			}
		}()

		logger.Info("Resource watcher is running")

		// Wait for context cancellation or error
		select {
		case <-ctx.Done():
			logger.Info("Shutting down resource watcher")
		case err := <-errChan:
			logger.Error(err, "Component error")
			return err
		}

		return nil
	}
}
