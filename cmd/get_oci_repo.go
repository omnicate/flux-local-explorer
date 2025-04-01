package cmd

import (
	"fmt"

	"github.com/fluxcd/flux2/v2/pkg/printers"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/omnicate/flx/loader"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

var getOciRepoCmd = &cobra.Command{
	Use:     "oci-repo",
	Aliases: []string{"or", "oci"},
	Short:   "Flux OCIRepository resources (oci)",
	PreRunE: getCmd.PreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			getArgs.name = args[0]
		}
		if err := repoLoader.Load(
			filesys.MakeFsOnDisk(),
			rootArgs.fluxDir,
			"flux-system",
			func(_ *loader.Kustomization, gr *loader.GitRepository) bool {
				return false
			},
		); err != nil {
			return err
		}
		ks := filterResources(repoLoader.OciRepositories())
		sortResources(ks)

		if getArgs.format == "pretty" {
			headers := ociRepoHeaders(getArgs.allNamespaces)
			rows := ociRepoRows(getArgs.allNamespaces, ks)
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

		return fmt.Errorf("invalid output format %q", getArgs.format)
	},
}

func init() {
	getCmd.AddCommand(getOciRepoCmd)
}

func ociRepoHeaders(includeNamespace bool) []string {
	var headers []string
	if includeNamespace {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"URL",
		"Reference",
	}...)
}

func ociRepoRows(includeNamespace bool, ks []*loader.OCIRepository) [][]string {
	var rows [][]string
	for _, ks := range ks {
		var row []string
		if includeNamespace {
			row = append(row, ks.Namespace)
		}
		row = append(row, []string{
			ks.Name,
			ks.Spec.URL,
			formatOciReference(ks.Spec.Reference),
		}...)
		rows = append(rows, row)
	}
	return rows
}

func formatOciReference(ref *sourcev1b2.OCIRepositoryRef) string {
	if ref == nil {
		return ""
	}
	if ref.Digest != "" {
		return "Digest: " + ref.Digest
	}
	if ref.Tag != "" {
		return "Tag: " + ref.Tag
	}
	if ref.SemVer != "" {
		return "Version: " + ref.SemVer
	}
	return "Unknown"
}
