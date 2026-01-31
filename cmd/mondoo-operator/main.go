// Copyright Mondoo, Inc. 2026
// SPDX-License-Identifier: BUSL-1.1

package main

import (
	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/garbage_collect"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/k8s_scan"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/operator"
	"go.mondoo.com/mondoo-operator/cmd/mondoo-operator/version"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "mondoo-operator",
	Short: "Mondoo Operator CLI",
}

func main() {
	rootCmd.AddCommand(operator.Cmd, version.Cmd, garbage_collect.Cmd, k8s_scan.Cmd)

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
