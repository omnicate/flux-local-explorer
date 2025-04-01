package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/loader"
)

// getKustomizationCmd represents the getKustomization command
var getKustomizationCmd = &cobra.Command{
	Use:     "kustomization",
	Aliases: []string{"ks"},
	Short:   "Flux Kustomization resources (ks)",
	PreRunE: getCmd.PreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			getArgs.name = args[0]
		}
		seq := repoLoader.Kustomizations(
			filesys.MakeFsOnDisk(),
			rootArgs.fluxDir,
			"flux-system",
		)
		results, err := getResultsFromSeq(seq)
		if err != nil {
			return err
		}
		if getArgs.format == "kustomize" {
			for _, re := range results {
				for _, r := range re.Resources {
					fmt.Println("---")
					fmt.Println(r.MustYaml())
				}
			}
			return nil
		}
		return printResults(results, kustomizationHeaders, kustomizationRows)
	},
}

func init() {
	getCmd.AddCommand(getKustomizationCmd)
}

func kustomizationHeaders() []string {
	var headers []string
	if getArgs.allNamespaces {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"Source",
		"Resources",
		"Error",
	}...)
}

func kustomizationRows(ks *loader.Kustomization) []string {
	var row []string
	if getArgs.allNamespaces {
		row = append(row, ks.Namespace)
	}
	return append(row, []string{
		ks.Name,
		formatSource(ks),
		fmt.Sprintf("%d", len(ks.Resources)),
		errOrEmpty(ks.Error),
	}...)
}

func formatSource(ks *loader.Kustomization) string {
	ns := ks.Namespace
	if sourceNs := ks.Spec.SourceRef.Namespace; sourceNs != "" {
		ns = sourceNs
	}
	switch ks.Spec.SourceRef.Kind {
	case "GitRepository":
		return "git: " + ns + "/" + ks.Spec.SourceRef.Name
	case "OCIRepository":
		return "oci: " + ns + "/" + ks.Spec.SourceRef.Name
	}
	panic("unsupported kustomization source " + ks.Spec.SourceRef.Kind)
}
