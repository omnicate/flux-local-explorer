package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/fluxcd/flux2/v2/pkg/printers"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/omnicate/flx/loader/manager"
)

type GetFlags struct {
	namespace     string
	allNamespaces bool
	format        string
	name          string
}

var (
	repoLoader *manager.Loader
	getArgs    GetFlags
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

func sortResources[T manager.NamedResource](list []T) {
	sort.Slice(list, func(i, j int) bool {
		{
			a, b := list[i].GetNamespace(), list[j].GetNamespace()
			if a != b {
				return a < b
			}
		}
		{
			a, b := list[i].GetName(), list[j].GetName()
			if a != b {
				return a < b
			}
		}
		return false
	})
}

func getResultsFromSeq[T manager.NamedResource](
	resources []T,
) ([]T, error) {
	var results []T

	for _, res := range resources {
		if getArgs.allNamespaces {
			results = append(results, res)
			continue
		}
		if ns := getArgs.namespace; ns != "" && res.GetNamespace() != ns {
			continue
		}
		if name := getArgs.name; name != "" && res.GetName() != name {
			continue
		}
		results = append(results, res)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no resources")
	}
	sortResources(results)
	return results, nil
}

func printResults[T manager.NamedResource](
	results []T,
	headerFunc func() []string,
	rowFunc func(T) []string,
) error {
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
			data, _ := yaml.Marshal(r)
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
