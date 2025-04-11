package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/omnicate/flx/internal/diff"
)

// diffKustomizationCmd compares two kustomizations
var diffKustomizationCmd = &cobra.Command{
	Use:     "kustomization",
	Aliases: []string{"ks"},
	Short:   "Flux Kustomization resources (ks)",
	PreRunE: getCmd.PreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			diffArgs.name = args[0]
		}

		d := diff.New(os.TempDir(), diffArgs.command)

		{
			logger.Debug().Msg("Getting base version")
			mgr, err := newManager(false)
			if err != nil {
				return err
			}
			if err := mgr.Run(); err != nil {
				return err
			}
			results := mgr.ListWithKind(
				"Kustomization",
				diffArgs.namespace,
				diffArgs.allNamespaces,
			)
			results = filterResults(results, diffArgs.name, diffArgs.namespace, diffArgs.allNamespaces)
			for _, result := range results {
				for _, res := range result.GetResources() {
					d.AddBase(res.Resource)
				}
			}
		}

		{
			logger.Debug().Msg("Getting current version")
			mgr, err := newManager(true)
			if err != nil {
				return err
			}
			if err := mgr.Run(); err != nil {
				return err
			}
			results := mgr.ListWithKind(
				"Kustomization",
				diffArgs.namespace,
				diffArgs.allNamespaces,
			)
			results = filterResults(results, diffArgs.name, diffArgs.namespace, diffArgs.allNamespaces)
			for _, result := range results {
				for _, res := range result.GetResources() {
					d.AddAgainst(res.Resource)
				}
			}
		}

		diffOutput, err := d.PrettyDiffAll()
		if err != nil {
			return err
		}

		fmt.Println(diffOutput)
		return nil
	},
}

func init() {
	diffCmd.AddCommand(diffKustomizationCmd)
}
