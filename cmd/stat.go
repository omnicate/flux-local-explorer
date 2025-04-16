package cmd

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/omnicate/flx/internal/loader"
)

var statCmd = &cobra.Command{
	Use:     "stat",
	Short:   "Flux Kustomization resources (ks)",
	PreRunE: getCmd.PreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			getArgs.name = args[0]
		}
		mgr, err := newManager(true)
		if err != nil {
			return err
		}
		if err := mgr.Run(); err != nil {
			return err
		}
		results := mgr.AllNodes()

		sort.Slice(results, func(i, j int) bool {
			return results[i].Duration < results[j].Duration
		})

		return printResults(results, statHeaders, statRows)
	},
}

func init() {
	rootCmd.AddCommand(statCmd)
}

func statHeaders() []string {
	var headers []string
	if getArgs.allNamespaces {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"Kind",
		"Duration",
	}...)
}

func statRows(rn *loader.ResourceNode) []string {
	var row []string
	if getArgs.allNamespaces {
		row = append(row, rn.Resource.GetNamespace())
	}

	return append(row, []string{
		rn.Resource.GetName(),
		rn.Resource.GetKind(),
		rn.Duration.String(),
	}...)
}
