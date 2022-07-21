package k8s_scan

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"

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

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		log.SetLogger(zap.New())
		logger := log.Log.WithName("k8s-scan")

		if *scanApiUrl == "" {
			return fmt.Errorf("--scan-api-url must be provided")
		}
		if *tokenFilePath == "" {
			return fmt.Errorf("--token-file-path must be provided")
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
		res, err := client.ScanKubernetesResources(context.Background(), *integrationMrn, *scanContainerImages)
		if err != nil {
			logger.Error(err, "failed to trigger a Kubernetes resources scan")
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
