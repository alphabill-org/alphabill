package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type (
	alphabillApp struct {
		baseCmd    *cobra.Command
		baseConfig *baseConfiguration
		opts       interface{}
	}
)

// New creates a new Alphabill application
func New() *alphabillApp {
	baseCmd, baseConfig := newBaseCmd()
	return &alphabillApp{baseCmd, baseConfig, nil}
}

func (a *alphabillApp) WithOpts(opts interface{}) *alphabillApp {
	a.opts = opts
	return a
}

// Execute adds all child commands and runs the application
func (a *alphabillApp) Execute(ctx context.Context) {
	cobra.CheckErr(a.addAndExecuteCommand(ctx))
}

func (a *alphabillApp) addAndExecuteCommand(ctx context.Context) error {
	a.baseCmd.AddCommand(newMoneyNodeCmd(a.baseConfig, convertOptsToRunnable(a.opts)))
	a.baseCmd.AddCommand(newMoneyGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newVDNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newVDGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newWalletCmd(a.baseConfig))
	a.baseCmd.AddCommand(newRootGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newRootChainCmd(a.baseConfig))
	a.baseCmd.AddCommand(newNodeIdentifierCmd())
	a.baseCmd.AddCommand(newVDClientCmd(a.baseConfig))
	a.baseCmd.AddCommand(newTokensNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newUserTokenGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newMoneyBackendCmd(a.baseConfig))
	a.baseCmd.AddCommand(newTokenWalletBackendCmd(a.baseConfig))
	return a.baseCmd.ExecuteContext(ctx)
}

func newBaseCmd() (*cobra.Command, *baseConfiguration) {
	config := &baseConfiguration{}
	// baseCmd represents the base command when called without any subcommands
	var baseCmd = &cobra.Command{
		Use:   "alphabill",
		Short: "The alphabill CLI",
		Long:  `The alphabill CLI includes commands for all different parts of the system: shard, core, wallet etc.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// You can bind cobra and viper in a few locations, but PersistencePreRunE on the base command works well
			// If subcommand does not define PersistentPreRunE, the one from base cmd is used.
			err := initializeConfig(cmd, config)
			if err != nil {
				return err
			}
			initializeLogger(config)
			return nil
		},
	}
	config.addConfigurationFlags(baseCmd)

	return baseCmd, config
}

// initializeConfig reads in config file and ENV variables if set.
func initializeConfig(cmd *cobra.Command, config *baseConfiguration) error {
	v := viper.New()

	config.initConfigFileLocation()

	if config.configFileExists() {
		v.SetConfigFile(config.CfgFile)
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

func initializeLogger(config *baseConfiguration) {
	loggerConfigFile := config.LogCfgFile
	if !strings.HasPrefix(config.LogCfgFile, string(os.PathSeparator)) {
		// Logger config file URL is using relative path
		loggerConfigFile = path.Join(config.HomeDir, config.LogCfgFile)
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
