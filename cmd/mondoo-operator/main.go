package main

import (
	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/k8s_scan"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/operator"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/version"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/webhook"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mondoo-operator",
	Short: "Mondoo Operator CLI",
}

func main() {
	rootCmd.AddCommand(operator.Cmd, webhook.Cmd, version.Cmd, k8s_scan.Cmd)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
