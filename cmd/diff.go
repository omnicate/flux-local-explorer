package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

type DiffFlags struct {
	name          string
	namespace     string
	allNamespaces bool

	command string
	short   bool
}

var diffArgs DiffFlags

// getCmd represents the get command
var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Diff two flux clusters",
	PreRunE: func(cmd *cobra.Command, args []string) error {
		bin, _, ok := strings.Cut(diffArgs.command, " ")
		if !ok {
			return fmt.Errorf("not a valid diff command: %s", diffArgs.command)
		}
		_, err := exec.LookPath(bin)
		return err
	},
}

func init() {
	diffCmd.PersistentFlags().BoolVarP(
		&diffArgs.allNamespaces,
		"all-namespaces",
		"A",
		false,
		"diff the requested object(s) across all namespaces",
	)
	diffCmd.PersistentFlags().StringVarP(
		&diffArgs.namespace,
		"namespace",
		"n",
		"flux-system",
		"diff the requested object(s) in this namespace",
	)
	diffCmd.PersistentFlags().BoolVarP(
		&diffArgs.short,
		"short",
		"",
		false,
		"only print summary",
	)
	diffCmd.PersistentFlags().StringVarP(
		&diffArgs.command,
		"diff-tool",
		"",
		"dyff --color on between -b -i ${base} ${against}",
		"only print summary",
	)
	rootCmd.AddCommand(diffCmd)
}
