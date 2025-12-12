// Copyright (c) Mondoo, Inc.
// SPDX-License-Identifier: BUSL-1.1

package garbage_collect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"go.mondoo.com/cnspec/v12/policy/scan"
	"go.mondoo.com/mondoo-operator/pkg/client/scanapiclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var Cmd = &cobra.Command{
	Use:   "garbage-collect",
	Short: "Triggers garbage collection based on provided critera.",
}

func init() {
	scanApiUrl := Cmd.Flags().String("scan-api-url", "", "The URL of the service to send scan requests to.")
	tokenInput := Cmd.Flags().String("token", "", "The token to use when making requests to the scan API. Cannot be specified in combination with --token-file-path.")
	tokenFilePath := Cmd.Flags().String("token-file-path", "", "Path to a file containing token to use when making requests to the scan API. Cannot be specified in combination with --token.")
	timeout := Cmd.Flags().Int64("timeout", 0, "The timeout in minutes for the garbage collection request.")
	filterPlatformRuntime := Cmd.Flags().String("filter-platform-runtime", "", "Cleanup assets by an asset's PlatformRuntime.")
	filterManagedBy := Cmd.Flags().String("filter-managed-by", "", "Cleanup assets with matching ManagedBy field")
	filterOlderThan := Cmd.Flags().String("filter-older-than", "", "Cleanup assets which have not been updated in over the time provided (eg 12m or 48h or anything time.ParseDuration() accepts)")
	labelsInput := Cmd.Flags().StringSlice("labels", []string{}, "Cleanup assets with matching labels (eg --labels key1=value1,key2=value2)")
	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		logger := log.Log.WithName("garbage-collect")

		if *scanApiUrl == "" {
			return fmt.Errorf("--scan-api-url must be provided")
		}
		if *tokenFilePath == "" && *tokenInput == "" {
			return fmt.Errorf("either --token or --token-file-path must be provided")
		}
		if *tokenFilePath != "" && *tokenInput != "" {
			return fmt.Errorf("only one of --token or --token-file-path must be provided")
		}
		if *timeout <= 0 {
			return fmt.Errorf("--timeout must be greater than 0")
		}

		labels := make(map[string]string)
		for _, l := range *labelsInput {
			split := strings.Split(l, "=")
			if len(split) != 2 {
				return fmt.Errorf("invalid label provided %s. Labels should be in the form of key=value", l)
			}
			labels[split[0]] = split[1]
		}

		token := *tokenInput
		if *tokenFilePath != "" {
			tokenBytes, err := os.ReadFile(*tokenFilePath)
			if err != nil {
				logger.Error(err, "failed to read in file with token content")
				return err
			}
			token = strings.TrimSuffix(string(tokenBytes), "\n")
		}

		client, err := scanapiclient.NewClient(scanapiclient.ScanApiClientOptions{
			ApiEndpoint: *scanApiUrl,
			Token:       token,
			HttpTimeout: ptr.To(time.Duration((*timeout)) * time.Minute),
		})
		if err != nil {
			return err
		}

		logger.Info("triggering garbage collection")
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration((*timeout))*time.Minute)
		defer cancel()

		if *filterManagedBy == "" && *filterOlderThan == "" && *filterPlatformRuntime == "" {
			return fmt.Errorf("no filters provided to garbage collect by")
		}

		return GarbageCollectCmd(ctx, client, *filterPlatformRuntime, *filterOlderThan, *filterManagedBy, labels, logger)
	}
}

func GarbageCollectCmd(ctx context.Context, client scanapiclient.ScanApiClient, platformRuntime, olderThan, managedBy string, labels map[string]string, logger logr.Logger) error {
	gcOpts := &scan.GarbageCollectOptions{
		ManagedBy: managedBy,
		Labels:    labels,
	}

	if olderThan != "" {
		timestamp, err := buildOlderThanTimestamp(olderThan)
		if err != nil {
			logger.Error(err, fmt.Sprintf("failed to parse provided older-than parameter (%s) into RFC3339 timestamp", olderThan))
			return err
		}

		gcOpts.OlderThan = timestamp
	}

	if platformRuntime != "" {
		switch platformRuntime {
		case "k8s-cluster", "docker-image":
			gcOpts.PlatformRuntime = platformRuntime
		default:
			return fmt.Errorf("no matching platform runtime found for (%s)", platformRuntime)
		}
	}

	err := client.GarbageCollectAssets(ctx, gcOpts)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Error(err, "failed to receive a response before timeout was exceeded")
		} else {
			logger.Error(err, "error while performing garbage collection")
		}
		return err
	}

	return nil
}

func buildOlderThanTimestamp(olderThanString string) (string, error) {
	duration, err := time.ParseDuration(olderThanString)
	if err != nil {
		return "", err
	}

	return time.Now().Add(-duration).Format(time.RFC3339), nil
}
