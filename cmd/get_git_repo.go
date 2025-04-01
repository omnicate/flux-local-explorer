package cmd

import (
	"fmt"
	"strings"

	"github.com/fluxcd/flux2/v2/pkg/printers"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/omnicate/flx/loader"
	"sigs.k8s.io/kustomize/kyaml/filesys"
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
		if err := repoLoader.Load(
			filesys.MakeFsOnDisk(),
			rootArgs.fluxDir,
			"flux-system",
			func(_ *loader.Kustomization, gr *loader.GitRepository) bool {
				if gr == nil {
					return false
				}
				if getArgs.name != gr.Name {
					return false
				}
				if getArgs.namespace != gr.Namespace {
					return false
				}
				return true
			},
		); err != nil {
			return err
		}
		ks := filterResources(repoLoader.GitRepositories())
		sortResources(ks)

		if getArgs.format == "pretty" {
			headers := gitRepoHeaders(getArgs.allNamespaces)
			rows := gitRepoRows(getArgs.allNamespaces, ks)
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
	getCmd.AddCommand(getGitRepoCmd)
}

func gitRepoHeaders(includeNamespace bool) []string {
	var headers []string
	if includeNamespace {
		headers = append(headers, "Namespace")
	}
	return append(headers, []string{
		"Name",
		"URL",
		"Reference",
		"Includes",
	}...)
}

func gitRepoRows(includeNamespace bool, ks []*loader.GitRepository) [][]string {
	var rows [][]string
	for _, ks := range ks {
		var row []string
		if includeNamespace {
			row = append(row, ks.Namespace)
		}
		row = append(row, []string{
			ks.Name,
			ks.Spec.URL,
			formatGitRepoReference(ks.Spec.Reference),
			formatIncludes(ks.Spec.Include),
		}...)
		rows = append(rows, row)
	}
	return rows
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
