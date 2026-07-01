package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/omnicate/flux-local-explorer/internal/affected"
)

type AffectedFlags struct {
	files  []string
	format string
}

var affectedArgs AffectedFlags

var affectedCmd = &cobra.Command{
	Use:   "affected",
	Short: "Find Flux Kustomizations affected by changed files",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			affectedArgs.files = append(affectedArgs.files, args...)
		}
		if len(affectedArgs.files) == 0 {
			return fmt.Errorf("at least one --file or positional file is required")
		}
		targets, err := affected.Find(affected.Options{
			RepoRoot: rootArgs.fluxDir,
			Files:    affectedArgs.files,
		})
		if err != nil {
			return err
		}
		return printAffectedTargets(cmd.OutOrStdout(), targets, affectedArgs.format)
	},
}

func init() {
	affectedCmd.Flags().StringArrayVarP(
		&affectedArgs.files,
		"file",
		"f",
		nil,
		"changed file to inspect",
	)
	affectedCmd.Flags().StringVarP(
		&affectedArgs.format,
		"output",
		"o",
		"tsv",
		"output format, one of [tsv, yaml]",
	)
	rootCmd.AddCommand(affectedCmd)
}

func printAffectedTargets(w io.Writer, targets []affected.Target, format string) error {
	switch format {
	case "tsv":
		for _, target := range targets {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", target.EntryPoint, target.Namespace, target.Name); err != nil {
				return err
			}
		}
		return nil
	case "yaml":
		data, err := yaml.Marshal(targets)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(w, string(data))
		return err
	default:
		return fmt.Errorf("unknown output format: %s", format)
	}
}
