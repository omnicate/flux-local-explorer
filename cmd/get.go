package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/fluxcd/flux2/v2/pkg/printers"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/omnicate/flx/internal/loader"
)

type GetFlags struct {
	namespace     string
	allNamespaces bool
	format        string
	name          string
}

var (
	getArgs GetFlags
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Retrieve resources",
}

func init() {
	getCmd.PersistentFlags().BoolVarP(
		&getArgs.allNamespaces,
		"all-namespaces",
		"A",
		false,
		"list the requested object(s) across all namespaces",
	)
	getCmd.PersistentFlags().StringVarP(
		&getArgs.namespace,
		"namespace",
		"n",
		"flux-system",
		"list the requested object(s) in this namespace",
	)
	getCmd.PersistentFlags().StringVarP(
		&getArgs.format,
		"output-format",
		"o",
		"pretty",
		"format the list using one of [table, yaml]",
	)
	rootCmd.AddCommand(getCmd)
}

func sortResources(list []*loader.ResourceNode) {
	sort.Slice(list, func(i, j int) bool {
		{
			a, b := list[i].Resource.GetNamespace(), list[j].Resource.GetNamespace()
			if a != b {
				return a < b
			}
		}
		{
			a, b := list[i].Resource.GetName(), list[j].Resource.GetName()
			if a != b {
				return a < b
			}
		}
		return false
	})
}

func filterResults(
	resources []*loader.ResourceNode,
	filterName string,
	filterNamespace string,
	filterAllNamespaces bool,
) []*loader.ResourceNode {
	var results []*loader.ResourceNode

	for _, res := range resources {
		if filterAllNamespaces {
			results = append(results, res)
			continue
		}
		if ns := filterNamespace; ns != "" && res.Resource.GetNamespace() != ns {
			continue
		}
		if name := filterName; name != "" && res.Resource.GetName() != name {
			continue
		}
		results = append(results, res)
	}
	sortResources(results)
	return results
}

func printResults(
	results []*loader.ResourceNode,
	headerFunc func() []string,
	rowFunc func(node *loader.ResourceNode) []string,
) error {
	if len(results) == 0 {
		return fmt.Errorf("no results")
	}
	switch getArgs.format {
	case "pretty":
		var rows [][]string
		for _, item := range results {
			rows = append(rows, rowFunc(item))
		}
		return printers.TablePrinter(headerFunc()).Print(os.Stdout, rows)
	case "yaml":
		for _, r := range results {
			fmt.Println("---")
			data, _ := yaml.Marshal(r.Resource)
			fmt.Println(string(data))
		}
		return nil
	}
	return fmt.Errorf("unknown format: %s", getArgs.format)
}

func errOrEmpty(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
