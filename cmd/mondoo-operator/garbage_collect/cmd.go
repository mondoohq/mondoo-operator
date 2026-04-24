// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package garbage_collect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/pkg/client/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"go.mondoo.com/mondoo-operator/pkg/utils/mondoo"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var Cmd = &cobra.Command{
	Use:   "garbage-collect",
	Short: "Triggers garbage collection based on provided criteria.",
}

func init() {
	configPath := Cmd.Flags().String("config", "", "The path to the mondoo.yml config file containing service account credentials.")
	timeout := Cmd.Flags().Int64("timeout", 5, "The timeout in minutes for the garbage collection request.")
	filterPlatformRuntime := Cmd.Flags().String("filter-platform-runtime", "", "Cleanup assets by an asset's PlatformRuntime (k8s-cluster or docker-image).")
	filterManagedBy := Cmd.Flags().String("filter-managed-by", "", "Cleanup assets with matching ManagedBy field.")
	filterOlderThan := Cmd.Flags().String("filter-older-than", "", "Cleanup assets which have not been updated in over the time provided (eg 12m or 48h or anything time.ParseDuration() accepts).")
	scopeMrnOverride := Cmd.Flags().String("scope-mrn", "", "Override the scope MRN (space or org) for garbage collection.")
	Cmd.Flags().String("space-mrn", "", "Deprecated: use --scope-mrn instead.")
	_ = Cmd.Flags().MarkDeprecated("space-mrn", "use --scope-mrn instead")
	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		logger := log.Log.WithName("garbage-collect")

		if *configPath == "" {
			return fmt.Errorf("--config must be provided")
		}
		if *timeout <= 0 {
			return fmt.Errorf("--timeout must be greater than 0")
		}

		// Read the service account credentials from the config file
		configData, err := os.ReadFile(*configPath)
		if err != nil {
			logger.Error(err, "failed to read config file")
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

		client, err := mondooclient.NewClient(mondooclient.MondooClientOptions{
			ApiEndpoint: serviceAccount.ApiEndpoint,
			Token:       token,
			HttpTimeout: ptr.To(time.Duration(*timeout) * time.Minute),
		})
		if err != nil {
			return err
		}

		logger.Info("triggering garbage collection")
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Minute)
		defer cancel()

		if *filterManagedBy == "" && *filterOlderThan == "" && *filterPlatformRuntime == "" {
			return fmt.Errorf("no filters provided to garbage collect by")
		}

		scopeMrn := *scopeMrnOverride
		if scopeMrn == "" {
			if spaceMrn, _ := cmd.Flags().GetString("space-mrn"); spaceMrn != "" {
				scopeMrn = spaceMrn
			}
		}
		if scopeMrn == "" {
			scopeMrn = serviceAccount.ScopeMrn
		}
		if scopeMrn == "" {
			scopeMrn = serviceAccount.SpaceMrn
		}
		return GarbageCollectCmd(ctx, client, scopeMrn, *filterPlatformRuntime, *filterOlderThan, *filterManagedBy, logger)
	}
}

func GarbageCollectCmd(ctx context.Context, client mondooclient.MondooClient, scopeMrn, platformRuntime, olderThan, managedBy string, logger logr.Logger) error {
	req := &mondooclient.GarbageCollectAssetsRequest{
		ScopeMrn:  scopeMrn,
		ManagedBy: managedBy,
	}

	if olderThan != "" {
		timestamp, err := buildOlderThanTimestamp(olderThan)
		if err != nil {
			logger.Error(err, fmt.Sprintf("failed to parse provided older-than parameter (%s) into RFC3339 timestamp", olderThan))
			return err
		}

		req.DateFilter = &mondooclient.DateFilter{
			Timestamp:  timestamp,
			Comparison: mondooclient.Comparison_LESS_THAN,
			Field:      mondooclient.DateFilterField_FILTER_LAST_UPDATED,
		}
	}

	if platformRuntime != "" {
		switch platformRuntime {
		case "k8s-cluster", "docker-image":
			req.PlatformRuntime = platformRuntime
		default:
			return fmt.Errorf("no matching platform runtime found for (%s)", platformRuntime)
		}
	}

	if err := client.GarbageCollectAssets(ctx, req); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Error(err, "failed to receive a response before timeout was exceeded")
		} else {
			logger.Error(err, "error while performing garbage collection")
		}
		return err
	}

	logger.Info("Garbage collection complete")
	return nil
}

func buildOlderThanTimestamp(olderThanString string) (string, error) {
	duration, err := time.ParseDuration(olderThanString)
	if err != nil {
		return "", err
	}

	return time.Now().Add(-duration).Format(time.RFC3339), nil
}
