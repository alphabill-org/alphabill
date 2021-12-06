package cmd

import (
	"fmt"
	"os"
	"strings"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type (
	alphabillApp struct {
		rootCmd    *cobra.Command
		rootConfig *rootConfiguration
	}
	rootConfiguration struct {
		// The Alphabill home directory
		HomeDir string
		// Configuration file URL. If it's relative, then it's relative from the HomeDir.
		CfgFile string
	}
)

const (
	// The prefix for configuration keys inside environment.
	envPrefix = "AB"
	// The default name for config file.
	defaultConfigFile = "config.props"
	// The default home directory.
	defaultHomeDir = "$HOME/.alphabill"
)

// New creates a new Alphabill application
func New() *alphabillApp {
	rootCmd, rootConfig := newRootCmd()
	return &alphabillApp{rootCmd, rootConfig}
}

// Execute adds all child commands and runs the application
func (a *alphabillApp) Execute(opts ...Option) {
	executeOpts := Options{}
	for _, o := range opts {
		o(&executeOpts)
	}

	a.rootCmd.AddCommand(newShardCmd(a.rootConfig, executeOpts.shardRunFunc))

	cobra.CheckErr(a.rootCmd.Execute())
}

func newRootCmd() (*cobra.Command, *rootConfiguration) {
	config := &rootConfiguration{}
	// rootCmd represents the base command when called without any subcommands
	var rootCmd = &cobra.Command{
		Use:   "alphabill",
		Short: "The alphabill CLI",
		Long:  `The alphabill CLI includes commands for all different parts of the system: shard, core, wallet etc.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// You can bind cobra and viper in a few locations, but PersistencePreRunE on the root command works well
			// If subcommand does not define PersistentPreRunE, the one from root cmd is used.
			return initializeConfig(cmd, config)
		},
	}
	rootCmd.PersistentFlags().StringVar(&config.HomeDir, "home", defaultHomeDir, "set the AB_HOME for this invocation (default is $HOME/.alphabill")
	rootCmd.PersistentFlags().StringVar(&config.CfgFile, "config", "", "config file location (default is $AB_HOME/config.yaml)")

	return rootCmd, config
}

// initConfig reads in config file and ENV variables if set.
func initializeConfig(cmd *cobra.Command, rootConfig *rootConfiguration) error {
	v := viper.New()

	if rootConfig.CfgFile == "" {
		rootConfig.CfgFile = defaultConfigFile
	}
	if !strings.HasPrefix(rootConfig.CfgFile, string(os.PathSeparator)) {
		// Relative path
		rootConfig.CfgFile = rootConfig.HomeDir + string(os.PathSeparator) + rootConfig.CfgFile
	}
	if fileExists(rootConfig.CfgFile) {
		v.SetConfigFile(rootConfig.CfgFile)
	}

	// Attempt to read the config file, gracefully ignoring errors
	// caused by a config file not being found. Return an error
	// if we cannot parse the config file.
	if err := v.ReadInConfig(); err != nil {
		// It's okay if there isn't a config file
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return err
		}
	}

	// When we bind flags to environment variables expect that the
	// environment variables are prefixed, e.g. a flag like --number
	// binds to an environment variable STING_NUMBER. This helps
	// avoid conflicts.
	v.SetEnvPrefix(envPrefix)

	// Bind to environment variables
	// Works great for simple config names, but needs help for names
	// like --favorite-color which we fix in the bindFlags function
	v.AutomaticEnv()

	// Bind the current command's flags to viper
	if err := bindFlags(cmd, v); err != nil {
		return errors.Wrap(err, "bind flags failed")
	}

	return nil
}

// Bind each cobra flag to its associated viper configuration (config file and environment variable)
func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	var bindFlagErr error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores, e.g. --favorite-color to STING_FAVORITE_COLOR
		if strings.Contains(f.Name, "-") {
			envVarSuffix := strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
			if err := v.BindEnv(f.Name, fmt.Sprintf("%s_%s", envPrefix, envVarSuffix)); err != nil {
				bindFlagErr = errors.Wrap(err, "could not bind env to cobra flag")
				return
			}
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			if err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				bindFlagErr = errors.Wrap(err, "could not set value to cobra flag")
				return
			}
		}
	})
	return bindFlagErr
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
	// P.S. the file might actually exist.
	// But in our case we consider the file missing.
}
