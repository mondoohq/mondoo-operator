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

		// TODO: I guess add integration-mrn label to the scans if it's available
		logger.Info("triggering Kubernetes resources scan")
		res, err := client.ScanKubernetesResources(context.Background())
		if err != nil {
			logger.Error(err, "failed to trigger a Kubernetes resources scan")
		}

		// TODO: print some more useful info
		if res.Ok {
			logger.Info("Kubernetes resources scan successful", "worst score", res.WorstScore.Value)
		} else {
			logger.Error(fmt.Errorf("scan API returned not OK. %v", res.WorstScore), "Kubernetes resources scan was not successful")
		}

		return nil
	}
}
