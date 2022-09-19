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
	"go.mondoo.com/mondoo-operator/pkg/garbagecollection"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	RUNTIME_KUBERNETES_CLUSTER = "k8s-cluster"
	RUNTIME_DOCKER_REGISTRY    = "docker-registry"
)

var Cmd = &cobra.Command{
	Use:   "garbage-collect",
	Short: "Triggers garbage collection based on provided critera.",
}

func init() {
	scanApiUrl := Cmd.Flags().String("scan-api-url", "", "The URL of the service to send scan requests to.")
	tokenFilePath := Cmd.Flags().String("token-file-path", "", "Path to a file containing token to use when making requests to the scan API.")
	timeout := Cmd.Flags().Int64("timeout", 0, "The timeout in minutes for the garbage collection request.")
	filterPlatformRuntime := Cmd.Flags().String("filter-platform-runtime", "", "Cleanup assets by an asset's PlatformRuntime.")
	filterManagedBy := Cmd.Flags().String("filter-managed-by", "", "Cleanup assets with matching ManagedBy field")
	filterOlderThan := Cmd.Flags().String("filter-older-than", "", "Cleanup assets which have not been updated in over the time provided (eg 12m or 48h or anything time.ParseDuration() accepts)")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		logger := log.Log.WithName("garbage-collect")

		if *scanApiUrl == "" {
			return fmt.Errorf("--scan-api-url must be provided")
		}
		if *tokenFilePath == "" {
			return fmt.Errorf("--token-file-path must be provided")
		}
		if *timeout <= 0 {
			return fmt.Errorf("--timeout must be greater than 0")
		}

		tokenBytes, err := os.ReadFile(*tokenFilePath)
		if err != nil {
			logger.Error(err, "failed to read in file with token content")
			return err
		}
		token := strings.TrimSuffix(string(tokenBytes), "\n")

		client := mondooclient.NewClient(mondooclient.ClientOptions{
			ApiEndpoint: *scanApiUrl,
			Token:       token,
		})

		logger.Info("triggering garbage collection")
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration((*timeout))*time.Minute)
		defer cancel()

		if *filterManagedBy == "" && *filterOlderThan == "" && *filterPlatformRuntime == "" {
			return fmt.Errorf("no filters provided to garbage collect by")
		}

		err = GarbageCollectCmd(ctx, client, *filterPlatformRuntime, *filterOlderThan, *filterManagedBy, logger)
		if err != nil {
			return err
		}

		return nil
	}
}

func GarbageCollectCmd(ctx context.Context, client mondooclient.Client, platformRuntime, olderThan, managedBy string, logger logr.Logger) error {
	gcOpts := &garbagecollection.GarbageCollectOptions{
		MangagedBy: managedBy,
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
		case RUNTIME_KUBERNETES_CLUSTER, RUNTIME_DOCKER_REGISTRY:
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
