package cmd

import (
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/spf13/cobra"

	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/omnicate/flx/resource"
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
		result, err := repoLoader.Load(
			filesys.MakeFsOnDisk(),
			rootArgs.fluxDir,
			"flux-system",
		)
		if err != nil {
			return err
		}
		results, err := getResultsFromSeq(result.OCIRepositories)
		if err != nil {
			return err
		}
		return printResults(results, ociRepoHeaders, ociRepoRows)
	},
}

func init() {
	getCmd.AddCommand(getOciRepoCmd)
}

func ociRepoHeaders() []string {
	var headers []string
	if getArgs.allNamespaces {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"URL",
		"Reference",
		"Error",
	}...)
}

func ociRepoRows(or *resource.OCIRepository) []string {
	var row []string
	if getArgs.allNamespaces {
		row = append(row, or.Namespace)
	}
	row = append(row, []string{
		or.Name,
		or.Spec.URL,
		formatOciReference(or.Spec.Reference),
		errOrEmpty(or.Error),
	}...)
	return row
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
