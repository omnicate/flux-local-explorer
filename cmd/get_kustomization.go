package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/fluxcd/flux2/v2/pkg/printers"

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
		if err := repoLoader.Load(
			filesys.MakeFsOnDisk(),
			rootArgs.fluxDir,
			"flux-system",
			func(ks *loader.Kustomization, _ *loader.GitRepository) bool {
				if ks == nil {
					return false
				}
				if getArgs.name != ks.Name {
					return false
				}
				if getArgs.namespace != ks.Namespace {
					return false
				}
				return true
			},
		); err != nil {
			return err
		}

		ks := filterResources(repoLoader.Kustomizations())
		sortResources(ks)

		if getArgs.format == "pretty" {
			headers := kustomizationHeaders(getArgs.allNamespaces)
			rows := kustomizationRows(getArgs.allNamespaces, ks)
			return printers.TablePrinter(headers).Print(cmd.OutOrStdout(), rows)
		}

		if getArgs.format == "yaml" {
			for _, r := range ks {
				fmt.Println("---")
				data, _ := yaml.Marshal(r)
				fmt.Println(string(data))
			}
			return nil
		}

		if getArgs.format == "kustomize" {
			if len(ks) == 0 {
				return fmt.Errorf("no resources")
			}
			for _, re := range ks {
				for _, r := range re.Resources {
					fmt.Println("---")
					fmt.Println(r.MustYaml())
				}
			}
			return nil
		}
		return fmt.Errorf("invalid output format %q", getArgs.format)
	},
}

func init() {
	getCmd.AddCommand(getKustomizationCmd)
}

func kustomizationHeaders(includeNamespace bool) []string {
	var headers []string
	if includeNamespace {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"Source",
		"Resources",
		"Error",
	}...)
}

func kustomizationRows(includeNamespace bool, ks []*loader.Kustomization) [][]string {
	var rows [][]string
	for _, ks := range ks {
		var row []string
		if includeNamespace {
			row = append(row, ks.Namespace)
		}
		row = append(row, []string{
			ks.Name,
			formatSource(ks),
			fmt.Sprintf("%d", len(ks.Resources)),
		}...)
		if ks.Error != nil {
			row = append(row, ks.Error.Error())
		}
		rows = append(rows, row)
	}
	return rows
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
