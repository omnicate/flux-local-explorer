package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/fluxcd/flux2/v2/pkg/printers"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/omnicate/flx/loader"
)

type GetFlags struct {
	namespace     string
	allNamespaces bool
	format        string
	name          string
}

var (
	repoLoader *loader.Loader
	getArgs    GetFlags
)

// getCmd represents the get command
var getCmd = &cobra.Command{
	Use:   "get",
	Short: "Retrieve resources",
	PreRunE: func(cmd *cobra.Command, args []string) error {

		level := zerolog.InfoLevel
		if rootArgs.verbose {
			level = zerolog.DebugLevel
		}
		output := io.Writer(zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
		})
		if rootArgs.logFormat == "json" {
			output = os.Stderr
		}
		logger := zerolog.New(output).Level(level).With().Timestamp().Logger()

		opts := []loader.Option{
			loader.WithLocalRepoRef(&loader.LocalGitRepository{
				Remote: rootArgs.localRemote,
				Path:   rootArgs.localPath,
				Branch: "master",
			}),
			loader.WithLogger(logger),
			loader.WithRepoCachePath(rootArgs.cacheDir),
		}
		repoLoader = loader.NewLoader(opts...)
		return nil
	},
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

func sortResources[T loader.NamedResource](list []T) {
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

func getResultsFromSeq[T loader.NamedResource](
	seq loader.ErrSeq[T],
) ([]T, error) {
	var results []T
	if getArgs.namespace != "" && getArgs.name != "" {
		ks, err := seq.Find(func(item T) bool {
			ns, name := item.GetNamespace(), item.GetName()
			return ns == getArgs.namespace && name == getArgs.name
		})
		if err != nil {
			return nil, err
		}
		results = append(results, ks)
	} else {
		ks, err := seq.Filter(func(item T) bool {
			if getArgs.allNamespaces {
				return true
			}
			return getArgs.namespace == item.GetNamespace()
		}).Collect()
		if err != nil {
			return nil, err
		}
		results = append(results, ks...)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no resources")
	}
	sortResources(results)
	return results, nil
}

func printResults[T loader.NamedResource](
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
