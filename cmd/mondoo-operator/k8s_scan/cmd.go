package k8s_scan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mondoo.com/cnquery/motor/providers"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/garbage_collect"
	"go.mondoo.com/mondoo-operator/pkg/feature_flags"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"go.mondoo.com/mondoo-operator/pkg/utils/logger"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var Cmd = &cobra.Command{
	Use:   "k8s-scan",
	Short: "Sends a requests for a Kubernetes resources scan to a scan API instance.",
}

func init() {
	scanApiUrl := Cmd.Flags().String("scan-api-url", "", "The URL of the service to send scan requests to.")
	tokenFilePath := Cmd.Flags().String("token-file-path", "", "Path to a file containing token to use when making scan requests.")
	integrationMrn := Cmd.Flags().String("integration-mrn", "", "The Mondoo integration MRN to label scanned items with if the MondooAuditConfig is configured with Mondoo integration.")
	scanContainerImages := Cmd.Flags().Bool("scan-container-images", false, "A value indicating whether to scan container images.")
	timeout := Cmd.Flags().Int64("timeout", 0, "The timeout in minutes for the scan request.")
	setManagedBy := Cmd.Flags().String("set-managed-by", "", "String to set the ManagedBy field for scanned/discovered assets")
	cleanupOlderThan := Cmd.Flags().String("cleanup-assets-older-than", "", "Set the age for which assets which have not been updated in over the time provided should be garbage collected (eg 12m or 48h)")
	includeNamespaces := Cmd.Flags().StringSlice("namespaces", nil, "Only resources residing in this list of Namespaces will be scanned")
	excludeNamespaces := Cmd.Flags().StringSlice("namespaces-exclude", nil, "Ignore resources residing in any of the specified Namespaces")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(logger.NewLogger())
		logger := log.Log.WithName("k8s-scan")

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

		logger.Info("triggering Kubernetes resources scan")
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration((*timeout))*time.Minute)
		defer cancel()
		scanOpts := &mondooclient.ScanKubernetesResourcesOpts{
			IntegrationMrn:      *integrationMrn,
			ScanContainerImages: *scanContainerImages,
			ManagedBy:           *setManagedBy,
			IncludeNamespaces:   *includeNamespaces,
			ExcludeNamespaces:   *excludeNamespaces,
		}
		res, err := client.ScanKubernetesResources(ctx, scanOpts)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				logger.Error(err, "failed to receive a response before the timeout was exceeded", "timeout", *timeout)
			} else {
				logger.Error(err, "failed to trigger a Kubernetes resources scan")
			}
			return err
		}

		// TODO: print some more useful info
		if res.Ok {
			logger.Info("Kubernetes resources scan successful", "worst score", res.WorstScore.Value)
		} else {
			err := fmt.Errorf("scan API returned not OK. %+v", res)
			logger.Error(err, "Kubernetes resources scan was not successful")
			return err
		}

		// If scanning successful, now attempt some cleanup of older assets
		if feature_flags.GetEnableGarbageCollection() && *setManagedBy != "" && *cleanupOlderThan != "" {
			platformRuntime := providers.RUNTIME_KUBERNETES_CLUSTER
			if *scanContainerImages {
				platformRuntime = providers.RUNTIME_DOCKER_IMAGE
			}

			err = garbage_collect.GarbageCollectCmd(ctx, client, platformRuntime, *cleanupOlderThan, *setManagedBy, make(map[string]string), logger)
			if err != nil {
				logger.Error(err, "error while garbage collecting assets; will attempt on next scan")
			}
		}

		return nil
	}
}
