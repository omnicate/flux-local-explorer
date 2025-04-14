package cmd

import (
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/spf13/cobra"

	"github.com/omnicate/flx/internal/loader"
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
		mgr, err := newManager(true)
		if err != nil {
			return err
		}
		if err := mgr.Run(); err != nil {
			return err
		}
		results := mgr.ListWithKind(
			"OCIRepository",
			getArgs.namespace,
			getArgs.allNamespaces,
		)
		results = filterResults(results, getArgs.name, getArgs.namespace, getArgs.allNamespaces)
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

func ociRepoRows(rn *loader.ResourceNode) []string {
	var or sourcev1b2.OCIRepository
	rn.Resource.Unmarshal(&or)

	var row []string
	if getArgs.allNamespaces {
		row = append(row, or.Namespace)
	}
	row = append(row, []string{
		or.Name,
		or.Spec.URL,
		formatOciReference(or.Spec.Reference),
		errOrEmpty(rn.Error),
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
