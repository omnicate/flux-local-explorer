package cmd

import (
	"strings"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/spf13/cobra"

	"github.com/omnicate/flx/loader/kube"
)

var getGitRepoCmd = &cobra.Command{
	Use:     "git-repo",
	Aliases: []string{"gr", "git"},
	Short:   "Flux GitRepository resources (gr)",
	PreRunE: getCmd.PreRunE,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			getArgs.name = args[0]
		}
		if err := repoLoader.Run(); err != nil {
			return err
		}
		results := repoLoader.ListWithKind(
			"GitRepository",
			getArgs.namespace,
			getArgs.allNamespaces,
		)
		results = filterResults(results)
		return printResults(results, gitRepoHeaders, gitRepoRows)
	},
}

func init() {
	getCmd.AddCommand(getGitRepoCmd)
}

func gitRepoHeaders() []string {
	var headers []string
	if getArgs.allNamespaces {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"URL",
		"Reference",
		"Includes",
		"Error",
	}...)
}

func gitRepoRows(rn *kube.ResourceNode) []string {
	var gr sourcev1.GitRepository
	rn.Resource.Unmarshal(&gr)
	var row []string
	if getArgs.allNamespaces {
		row = append(row, gr.Namespace)
	}
	return append(row, []string{
		gr.Name,
		gr.Spec.URL,
		formatGitRepoReference(gr.Spec.Reference),
		formatIncludes(gr.Spec.Include),
		errOrEmpty(rn.Error),
	}...)
}

func formatGitRepoReference(ref *sourcev1.GitRepositoryRef) string {
	if ref == nil {
		return ""
	}
	if ref.Commit != "" {
		return "Commit: " + ref.Commit
	}
	if ref.Branch != "" {
		return "Branch: " + ref.Branch
	}
	if ref.Tag != "" {
		return "Tag: " + ref.Tag
	}
	return "Unknown"
}

func formatIncludes(incls []sourcev1.GitRepositoryInclude) string {
	var repos []string
	for _, incl := range incls {
		repos = append(repos, incl.GitRepositoryRef.Name)
	}
	return strings.Join(repos, ", ")
}
