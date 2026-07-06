package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/omnicate/flux-local-explorer/internal/affected"
)

type ListFlags struct {
	format string
}

var listArgs ListFlags

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Flux inventory",
}

var listKustomizationCmd = &cobra.Command{
	Use:     "kustomization",
	Aliases: []string{"ks"},
	Short:   "List Flux Kustomization render targets",
	RunE: func(cmd *cobra.Command, args []string) error {
		targets, err := affected.List(affected.Options{
			RepoRoot: rootArgs.fluxDir,
		})
		if err != nil {
			return err
		}
		return printListTargets(cmd.OutOrStdout(), targets, listArgs.format)
	},
}

func init() {
	listCmd.PersistentFlags().StringVarP(
		&listArgs.format,
		"output",
		"o",
		"tsv",
		"output format, one of [tsv, yaml]",
	)
	listCmd.AddCommand(listKustomizationCmd)
	rootCmd.AddCommand(listCmd)
}

func printListTargets(w io.Writer, targets []affected.Target, format string) error {
	return printAffectedTargets(w, targets, format)
}
