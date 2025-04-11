package cmd

import (
	"fmt"
	"time"

	kustomizev1 "github.com/fluxcd/kustomize-controller/api/v1"
	"github.com/spf13/cobra"

	"github.com/omnicate/flx/loader/kube"
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

		start := time.Now()
		if err := repoLoader.Run(); err != nil {
			return err
		}
		logger.Debug().Str("elapsed", time.Since(start).String()).Msg("done")

		results := repoLoader.ListWithKind(
			"Kustomization",
			getArgs.namespace,
			getArgs.allNamespaces,
		)

		results = filterResults(results)

		if getArgs.format == "kustomize" {
			for _, re := range results {
				for _, r := range re.GetResources() {
					fmt.Println("---")
					fmt.Println(r.Resource.MustYaml())
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

func kustomizationRows(rn *kube.ResourceNode) []string {
	var row []string
	if getArgs.allNamespaces {
		row = append(row, rn.Resource.GetNamespace())
	}
	var ks kustomizev1.Kustomization
	rn.Resource.Unmarshal(&ks)

	return append(row, []string{
		ks.Name,
		formatSource(&ks),
		fmt.Sprintf("%d", len(rn.GetResources())),
		errOrEmpty(rn.Error),
	}...)
}

func formatSource(ks *kustomizev1.Kustomization) string {
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
