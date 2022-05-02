package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type (
	alphabillApp struct {
		rootCmd        *cobra.Command
		rootConfig     *rootConfiguration
		opts           interface{}
		cmdInterceptor func(*cobra.Command)
	}
)

// New creates a new Alphabill application
func New() *alphabillApp {
	rootCmd, rootConfig := newRootCmd()
	return &alphabillApp{rootCmd, rootConfig, nil, nil}
}

func (a *alphabillApp) WithOpts(opts interface{}) *alphabillApp {
	a.opts = opts
	return a
}

// Execute adds all child commands and runs the application
func (a *alphabillApp) Execute(ctx context.Context) {
	a.rootCmd.AddCommand(newMoneyShardCmd(ctx, a.rootConfig, convertOptsToRunnable(a.opts)))
	a.rootCmd.AddCommand(newVDShardCmd(ctx, a.rootConfig))
	a.rootCmd.AddCommand(newWalletCmd(ctx, a.rootConfig))

	if a.cmdInterceptor != nil {
		a.cmdInterceptor(a.rootCmd)
	}

	cobra.CheckErr(a.rootCmd.Execute())
}

func (a *alphabillApp) withCmdInterceptor(fn func(*cobra.Command)) *alphabillApp {
	a.cmdInterceptor = fn
	return a
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
			err := initializeConfig(cmd, config)
			if err != nil {
				return err
			}
			initializeLogger(config)
			return nil
		},
	}
	config.addConfigurationFlags(rootCmd)

	return rootCmd, config
}

// initializeConfig reads in config file and ENV variables if set.
func initializeConfig(cmd *cobra.Command, rootConfig *rootConfiguration) error {
	v := viper.New()

	rootConfig.initConfigFileLocation()

	if rootConfig.configFileExists() {
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
	// binds to an environment variable AB_NUMBER. This helps
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

func initializeLogger(config *rootConfiguration) {
	loggerConfigFile := config.LogCfgFile
	if !strings.HasPrefix(config.LogCfgFile, string(os.PathSeparator)) {
		// Logger config file URL is using relative path
		loggerConfigFile = config.HomeDir + string(os.PathSeparator) + config.LogCfgFile
	}

	err := logger.UpdateGlobalConfigFromFile(loggerConfigFile)
	if err != nil {
		if errors.ErrorCausedBy(err, errors.ErrFileNotFound) {
			// In a common case when the config file is not found, the error message is made shorter. Not to spam the log.
			log.Debug("The logger configuration file (%s) not found", loggerConfigFile)
		} else {
			log.Warning("Updating logger configuration failed. Error: %s", err.Error())
		}
	} else {
		log.Trace("Updating logger configuration succeeded.")
	}
}

// Bind each cobra flag to its associated viper configuration (config file and environment variable)
func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	var bindFlagErr error
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Name == keyHome || f.Name == keyConfig {
			// "home" and "config" are special configuration values, handled separately.
			return
		}

		// Environment variables can't have dashes in them, so bind them to their equivalent
		// keys with underscores, e.g. --favorite-color to AB_FAVORITE_COLOR
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
