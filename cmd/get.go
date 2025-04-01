package cmd

import (
	"io"
	"os"
	"sort"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

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

type namedResource interface {
	GetName() string
	GetNamespace() string
}

func filterResources[T namedResource](list []T) []T {
	var filtered []T
	for _, ks := range list {
		if !getArgs.allNamespaces && getArgs.namespace != "" && getArgs.namespace != ks.GetNamespace() {
			continue
		}
		if getArgs.name != "" && getArgs.name != ks.GetName() {
			continue
		}
		filtered = append(filtered, ks)
	}
	return filtered
}

func sortResources[T namedResource](list []T) {
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
