package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	ctrl "github.com/omnicate/flx/loader/controller"
	"github.com/omnicate/flx/loader/controller/extsecret"
	"github.com/omnicate/flx/loader/controller/git"
	"github.com/omnicate/flx/loader/controller/helm"
	"github.com/omnicate/flx/loader/controller/kustomize"
	"github.com/omnicate/flx/loader/controller/oci"
	"github.com/omnicate/flx/loader/kube"
)

type RootFlags struct {
	fluxDir string

	// Local working repo:
	localRemote string
	localPath   string
	localBranch string

	// Git options
	gitForceHTTPS bool

	// Misc options:
	verbose   bool
	logFormat string
	cacheDir  string
}

var (
	logger     zerolog.Logger
	rootArgs   RootFlags
	repoLoader *kube.Manager
)

var rootCmd = &cobra.Command{
	Use:   "flx",
	Short: "Offline Flux companion.",
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

		var err error
		rootArgs.localRemote, err = repoURL(cmd.Flag("dir").Value.String())
		if err != nil {
			return err
		}
		rootArgs.localPath, err = repoTopLevel(cmd.Flag("dir").Value.String())
		if err != nil {
			return err
		}
		rootArgs.localBranch, err = repoDefaultBranch(cmd.Flag("dir").Value.String())
		if err != nil {
			return fmt.Errorf("failed to determine default branch: %v", err)
		}

		logger.Debug().
			Str("remote", rootArgs.localRemote).
			Str("path", rootArgs.localPath).
			Str("branch", rootArgs.localBranch).
			Msg("using local git repository")

		repoLoader = kube.NewManager([]ctrl.Controller{
			git.NewController(
				logger.With().Str("controller", "git").Logger(),
				git.Options{
					CachePath: rootArgs.cacheDir,
					UseHTTPS:  rootArgs.gitForceHTTPS,
					Local: []*git.LocalReplace{
						{
							Remote: rootArgs.localRemote,
							Path:   rootArgs.localPath,
							Branch: rootArgs.localBranch,
						},
					},
				},
			),
			kustomize.NewController(
				logger.With().Str("controller", "kustomize").Logger(),
			),
			helm.NewController(
				logger.With().Str("controller", "kustomize").Logger(),
				helm.Options{
					CachePath: rootArgs.cacheDir,
				},
			),
			extsecret.NewController(
				logger.With().Str("controller", "external-secrets").Logger(),
			),
			oci.NewController(
				logger.With().Str("controller", "oci").Logger(),
				oci.Options{
					CachePath: rootArgs.cacheDir,
				},
			),
		})

		return repoLoader.Initialize(
			filesys.MakeFsOnDisk(),
			rootArgs.fluxDir,
			"flux-system",
		)
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
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

	rootCmd.PersistentFlags().StringVarP(
		&rootArgs.cacheDir,
		"cache-dir",
		"",
		cacheDir,
		"cache location",
	)

	rootCmd.PersistentFlags().BoolVarP(
		&rootArgs.gitForceHTTPS,
		"git-force-https",
		"",
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

	rootCmd.PersistentFlags().StringVarP(
		&rootArgs.logFormat,
		"log-format",
		"",
		"pretty",
		"log format to use (pretty, json)",
	)

	cobra.OnInitialize(func() {
		viper.SetEnvPrefix("flx")
		viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
		viper.AutomaticEnv()
		postInitCommands(rootCmd.Commands())
	})

	viper.SetDefault("dir", ".")
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
