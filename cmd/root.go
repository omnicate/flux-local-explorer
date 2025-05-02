package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flx/internal/controller"
	"github.com/omnicate/flx/internal/controller/extsecret"
	"github.com/omnicate/flx/internal/controller/git"
	"github.com/omnicate/flx/internal/controller/helm"
	"github.com/omnicate/flx/internal/controller/kustomize"
	"github.com/omnicate/flx/internal/controller/oci"
	"github.com/omnicate/flx/internal/loader"
)

type RootFlags struct {
	fluxDir string

	// Local working repo:
	localPaths []string

	// Git options
	gitForceHTTPS bool

	// Misc options:
	verbose   bool
	logFormat string
	cacheDir  string

	enabledControllers []string
}

var (
	logger   zerolog.Logger
	rootArgs RootFlags
)

var rootCmd = &cobra.Command{
	Use:          "flx",
	Short:        "Offline Flux companion.",
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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
		logger = zerolog.New(output).Level(level).With().Timestamp().Logger()

		return nil
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {

	rootCmd.PersistentFlags().StringVarP(
		&rootArgs.fluxDir,
		"dir",
		"C",
		"",
		"git repository tracked by flux",
	)

	cacheDir := "./cache"
	homeDir, err := os.UserHomeDir()
	if err == nil {
		cacheDir = filepath.Join(homeDir, ".flx")
	}

	rootCmd.PersistentFlags().StringVar(
		&rootArgs.cacheDir,
		"cache-dir",
		cacheDir,
		"cache location",
	)

	rootCmd.PersistentFlags().StringArrayVarP(
		&rootArgs.localPaths,
		"local",
		"L",
		[]string{},
		"paths to local git repository overrides",
	)

	rootCmd.PersistentFlags().BoolVar(
		&rootArgs.gitForceHTTPS,
		"git-force-https",
		false,
		"force git clone via https",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&rootArgs.verbose,
		"verbose",
		"v",
		false,
		"verbose logging",
	)

	rootCmd.PersistentFlags().StringVar(
		&rootArgs.logFormat,
		"log-format",
		"pretty",
		"log format to use (pretty, json)",
	)

	rootCmd.PersistentFlags().StringSliceVar(
		&rootArgs.enabledControllers,
		"controllers",
		[]string{"ks", "git", "oci", "helm", "external-secrets"},
		"controllers to enable",
	)

	cobra.OnInitialize(func() {
		viper.SetEnvPrefix("flx")
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
		viper.AutomaticEnv()
		postInitCommands(rootCmd.Commands())
	})

	viper.SetDefault("dir", ".")
}

func newManager(useLocal bool) (*loader.Manager, error) {
	// Ensure deps in path:
	for _, bin := range []string{"git", "helm"} {
		if _, err := exec.LookPath(bin); err != nil {
			return nil, fmt.Errorf("%s was not found in path: %v", bin, err)
		}
	}

	if rootArgs.fluxDir == "" {
		return nil, fmt.Errorf("flux entrypoint directory must be set")
	} else {
		stat, err := os.Stat(rootArgs.fluxDir)
		if err != nil {
			return nil, fmt.Errorf("failed to stat entrypoint: %w", err)
		}
		if !stat.IsDir() {
			return nil, fmt.Errorf("flux entrypoint %s is not a directory", rootArgs.fluxDir)
		}
	}

	// Build using local repositories:
	var localRepos []*git.LocalReplace
	if useLocal {
		for _, localPath := range append(rootArgs.localPaths, rootArgs.fluxDir) {
			var remote, topLevelPath, defaultBranch string
			if strings.Contains(localPath, "=") {
				keyVals := strings.Split(localPath, ",")
				for _, kv := range keyVals {
					key, value, ok := strings.Cut(kv, "=")
					if ok {
						switch key {
						case "path":
							topLevelPath = value
						case "remote":
							remote = value
						case "branch":
							defaultBranch = value
						}
					}
				}
			} else {
				var err error
				remote, err = repoURL(localPath)
				if err != nil {
					return nil, fmt.Errorf("could not determine remote URL: %v", err)
				}
				topLevelPath, err = repoTopLevel(localPath)
				if err != nil {
					return nil, fmt.Errorf("could not determine top-level path: %v", err)
				}
				defaultBranch, err = repoDefaultBranch(localPath)
				if err != nil {
					return nil, fmt.Errorf("failed to determine default branch: %v", err)
				}
			}
			if remote == "" || topLevelPath == "" || defaultBranch == "" {
				return nil, fmt.Errorf("invalid remote, path or branch for %s", localPath)
			}

			localRepos = append(localRepos, &git.LocalReplace{
				Remote: remote,
				Path:   topLevelPath,
				Branch: defaultBranch,
			})
			logger.Debug().
				Str("remote", remote).
				Str("path", topLevelPath).
				Str("branch", defaultBranch).
				Msg("using local git repository")
		}
	}

	var controllers []ctrl.Controller
	if slices.Contains(rootArgs.enabledControllers, "git") {
		logger.Debug().Msg("enabling git controller")
		controllers = append(controllers, git.NewController(
			logger.With().Str("controller", "git").Logger(),
			git.Options{
				CachePath: rootArgs.cacheDir,
				UseHTTPS:  rootArgs.gitForceHTTPS,
				Local:     localRepos,
			},
		))
	}
	if slices.Contains(rootArgs.enabledControllers, "oci") {
		logger.Debug().Msg("enabling oci controller")
		controllers = append(controllers, oci.NewController(
			logger.With().Str("controller", "oci").Logger(),
			oci.Options{
				CachePath: rootArgs.cacheDir,
			},
		))
	}
	if slices.Contains(rootArgs.enabledControllers, "ks") {
		logger.Debug().Msg("enabling kustomize controller")
		controllers = append(controllers, kustomize.NewController(
			logger.With().Str("controller", "kustomize").Logger(),
		))
	}
	if slices.Contains(rootArgs.enabledControllers, "helm") {
		logger.Debug().Msg("enabling helm controller")
		controllers = append(controllers, helm.NewController(
			logger.With().Str("controller", "helm").Logger(),
			helm.Options{
				CachePath: rootArgs.cacheDir,
			},
		))
	}
	if slices.Contains(rootArgs.enabledControllers, "external-secrets") {
		logger.Debug().Msg("enabling external-secrets controller")
		controllers = append(controllers, extsecret.NewController(
			logger.With().Str("controller", "external-secrets").Logger(),
		))
	}

	repoLoader := loader.NewManager(
		logger,
		controllers,
	)
	if err := repoLoader.Initialize(
		filesys.MakeFsOnDisk(),
		rootArgs.fluxDir,
		"flux-system",
	); err != nil {
		return nil, err
	}

	return repoLoader, nil
}

func postInitCommands(commands []*cobra.Command) {
	for _, cmd := range commands {
		presetRequiredFlags(cmd)
		if cmd.HasSubCommands() {
			postInitCommands(cmd.Commands())
		}
	}
}

func presetRequiredFlags(cmd *cobra.Command) {
	_ = viper.BindPFlags(cmd.Flags())
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if viper.IsSet(f.Name) && viper.GetString(f.Name) != "" {
			_ = cmd.Flags().Set(f.Name, viper.GetString(f.Name))
		}
	})
}

func repoURL(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "config", "--get", "remote.origin.url")
	var buf bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func repoTopLevel(path string) (string, error) {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel")
	var buf bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func repoDefaultBranch(path string) (string, error) {
	cmd := exec.Command(
		"git", "-C", path, "remote", "show", "origin",
	)
	var buf bytes.Buffer
	cmd.Stdin = os.Stdin
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		branch, ok := strings.CutPrefix(line, "HEAD branch: ")
		if ok {
			return branch, nil
		}
	}
	return "", fmt.Errorf("default branch not in %s", buf.String())
}
