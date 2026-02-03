// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package k8s_scan

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/garbage_collect"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var Cmd = &cobra.Command{
	Use:   "k8s-scan",
	Short: "Scans Kubernetes resources and performs garbage collection of stale assets.",
}

func init() {
	configPath := Cmd.Flags().String("config", "", "The path to the mondoo.yml config file containing service account credentials.")
	inventoryFile := Cmd.Flags().String("inventory-file", "", "Path to the inventory.yml file for cnspec.")
	timeout := Cmd.Flags().Int64("timeout", 25, "The timeout in minutes for the scan request.")
	setManagedBy := Cmd.Flags().String("set-managed-by", "", "String to set the ManagedBy field for scanned/discovered assets.")
	cleanupOlderThan := Cmd.Flags().String("cleanup-assets-older-than", "", "Set the age for which assets which have not been updated in over the time provided should be garbage collected (eg 12m or 48h).")
	apiProxy := Cmd.Flags().String("api-proxy", "", "HTTP proxy to use for API requests.")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		logger := log.Log.WithName("k8s-scan")

		if *configPath == "" {
			return fmt.Errorf("--config must be provided")
		}
		if *inventoryFile == "" {
			return fmt.Errorf("--inventory-file must be provided")
		}
		if *timeout <= 0 {
			return fmt.Errorf("--timeout must be greater than 0")
		}

		// Build cnspec command
		cnspecArgs := []string{
			"scan", "k8s",
			"--config", *configPath,
			"--inventory-file", *inventoryFile,
			"--score-threshold", "0",
		}
		if *apiProxy != "" {
			cnspecArgs = append(cnspecArgs, "--api-proxy", *apiProxy)
		}

		logger.Info("executing cnspec scan", "args", cnspecArgs)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Minute)
		defer cancel()

		// Execute cnspec scan k8s
		cnspecCmd := exec.CommandContext(ctx, "cnspec", cnspecArgs...) //nolint:gosec // cnspec is a trusted binary
		cnspecCmd.Stdout = os.Stdout
		cnspecCmd.Stderr = os.Stderr
		cnspecCmd.Env = append(os.Environ(), "MONDOO_AUTO_UPDATE=false")

		if err := cnspecCmd.Run(); err != nil {
			logger.Error(err, "cnspec scan failed")
			return err
		}

		logger.Info("cnspec scan completed successfully")

		// If scanning successful, now attempt some cleanup of older assets
		if *setManagedBy != "" && *cleanupOlderThan != "" {
			logger.Info("starting garbage collection of stale assets")

			// Read the service account credentials from the config file
			configData, err := os.ReadFile(*configPath)
			if err != nil {
				logger.Error(err, "failed to read config file for garbage collection")
				return err
			}

			serviceAccount, err := mondoo.LoadServiceAccountFromFile(configData)
			if err != nil {
				logger.Error(err, "failed to parse service account from config file")
				return err
			}

			token, err := mondoo.GenerateTokenFromServiceAccount(*serviceAccount, logger)
			if err != nil {
				logger.Error(err, "failed to generate token from service account")
				return err
			}

			var httpProxy *string
			if *apiProxy != "" {
				httpProxy = apiProxy
			}

			client, err := mondooclient.NewClient(mondooclient.MondooClientOptions{
				ApiEndpoint: serviceAccount.ApiEndpoint,
				Token:       token,
				HttpProxy:   httpProxy,
				HttpTimeout: ptr.To(time.Duration(*timeout) * time.Minute),
			})
			if err != nil {
				logger.Error(err, "failed to create mondoo client")
				return err
			}

			// Garbage collect k8s-cluster assets
			platformRuntime := "k8s-cluster"
			logger.Info("garbage collecting assets", "platformRuntime", platformRuntime, "cleanupOlderThan", *cleanupOlderThan)

			err = garbage_collect.GarbageCollectCmd(ctx, client, platformRuntime, *cleanupOlderThan, *setManagedBy, make(map[string]string), logger)
			if err != nil {
				logger.Error(err, "error while garbage collecting assets; will attempt on next scan", "platform", platformRuntime)
				// Don't return error - GC failure shouldn't fail the overall scan
			}
		} else {
			logger.Info("skipping garbage collection of assets; either --set-managed-by or --cleanup-assets-older-than are missing")
		}

		return nil
	}
}
