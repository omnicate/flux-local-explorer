package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type RootFlags struct {
	fluxDir     string
	localRemote string
	localPath   string
	verbose     bool
	logFormat   string
	cacheDir    string
}

var rootArgs RootFlags

var rootCmd = &cobra.Command{
	Use:   "flx",
	Short: "A brief description of your application",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		rootArgs.localRemote, err = repoURL(cmd.Flag("dir").Value.String())
		if err != nil {
			return err
		}
		rootArgs.localPath, err = repoTopLevel(cmd.Flag("dir").Value.String())
		if err != nil {
			return err
		}
		return nil
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
	viper.BindPFlags(cmd.Flags())
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if viper.IsSet(f.Name) && viper.GetString(f.Name) != "" {
			cmd.Flags().Set(f.Name, viper.GetString(f.Name))
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
