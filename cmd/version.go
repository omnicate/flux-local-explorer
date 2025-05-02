package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "development"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Prints the version of flx",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Version: "+Version)
		return err
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
