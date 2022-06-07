package version

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/pkg/version"
)

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Displays the Mondoo Operator version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version.Version)
	},
}
