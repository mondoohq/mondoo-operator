package k8s_scan

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/pkg/mondooclient"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(zap.New())
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

		tokenBytes, err := ioutil.ReadFile(*tokenFilePath)
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
		res, err := client.ScanKubernetesResources(ctx, *integrationMrn, *scanContainerImages)
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
			err := fmt.Errorf("scan API returned not OK. %v", res.WorstScore)
			logger.Error(err, "Kubernetes resources scan was not successful")
			return err
		}

		return nil
	}
}
