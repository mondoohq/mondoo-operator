package version

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mondoo.com/mondoo-operator/pkg/version"
)

var Cmd = &cobra.Command{
	Use:   "version",
	Short: "Displays the Mondoo Operator version",
}

func init() {
	simple := Cmd.Flags().Bool("simple", false, "Shows only the version of the binary")

	Cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if *simple {
			fmt.Println(version.Version)
			return nil
		}

		fmt.Printf("Version: %s Commit: %s", version.Version, version.Commit)
		return nil
	}
}
