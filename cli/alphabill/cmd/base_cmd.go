package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
func New(logF LoggerFactory) *alphabillApp {
	baseCmd, baseConfig := newBaseCmd(logF)
	return &alphabillApp{baseCmd, baseConfig, nil}
}

func (a *alphabillApp) WithOpts(opts interface{}) *alphabillApp {
	a.opts = opts
	return a
}

// Execute adds all child commands and runs the application
func (a *alphabillApp) Execute(ctx context.Context) (err error) {
	defer func() {
		if a.baseConfig.observe != nil {
			err = errors.Join(err, a.baseConfig.observe.Shutdown())
		}
	}()

	return a.addAndExecuteCommand(ctx)
}

func (a *alphabillApp) addAndExecuteCommand(ctx context.Context) error {
	a.baseCmd.AddCommand(newMoneyNodeCmd(a.baseConfig, convertOptsToRunnable(a.opts)))
	a.baseCmd.AddCommand(newMoneyGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newWalletCmd(a.baseConfig))
	a.baseCmd.AddCommand(newRootGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newRootNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newNodeIdentifierCmd())
	a.baseCmd.AddCommand(newTokensNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newUserTokenGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newMoneyBackendCmd(a.baseConfig))
	a.baseCmd.AddCommand(newTokensBackendCmd(a.baseConfig))
	a.baseCmd.AddCommand(newEvmNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newEvmGenesisCmd(a.baseConfig))
	return a.baseCmd.ExecuteContext(ctx)
}

func newBaseCmd(logF LoggerFactory) (*cobra.Command, *baseConfiguration) {
	config := &baseConfiguration{loggerBuilder: logF}
	// baseCmd represents the base command when called without any subcommands
	var baseCmd = &cobra.Command{
		Use:           "alphabill",
		Short:         "The alphabill CLI",
		Long:          `The alphabill CLI includes commands for all different parts of the system: shard, core, wallet etc.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// You can bind cobra and viper in a few locations, but PersistencePreRunE on the base command works well
			// If subcommand does not define PersistentPreRunE, the one from base cmd is used.
			if err := initializeConfig(cmd, config); err != nil {
				return fmt.Errorf("failed to initialize configuration: %w", err)
			}
			return nil
		},
	}
	config.addConfigurationFlags(baseCmd)

	return baseCmd, config
}

func initializeConfig(cmd *cobra.Command, config *baseConfiguration) error {
	var errs []error

	if err := config.initializeConfig(cmd); err != nil {
		errs = append(errs, fmt.Errorf("reading configuration: %w", err))
	}

	if err := config.initLogger(cmd); err != nil {
		errs = append(errs, fmt.Errorf("initializing logger: %w", err))
	}

	metrics, err := cmd.Flags().GetString(keyMetrics)
	if err != nil {
		errs = append(errs, fmt.Errorf("reading flag %q: %w", keyMetrics, err))
	} else {
		obs, err := newObservability(metrics)
		if err != nil {
			errs = append(errs, fmt.Errorf("initializing observability: %w", err))
		}
		config.observe = obs
	}

	return errors.Join(errs...)
}

// initializeConfig reads in config file and ENV variables if set.
func (config *baseConfiguration) initializeConfig(cmd *cobra.Command) error {
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
		return fmt.Errorf("binding flags: %w", err)
	}

	return nil
}

// Bind each cobra flag to its associated viper configuration (config file and environment variable)
func bindFlags(cmd *cobra.Command, v *viper.Viper) error {
	var bindFlagErr []error
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
				bindFlagErr = append(bindFlagErr, fmt.Errorf("binding env to flag %q: %w", f.Name, err))
				return
			}
		}

		// Apply the viper config value to the flag when the flag is not set and viper has a value
		if !f.Changed && v.IsSet(f.Name) {
			val := v.Get(f.Name)
			if err := cmd.Flags().Set(f.Name, fmt.Sprintf("%v", val)); err != nil {
				bindFlagErr = append(bindFlagErr, fmt.Errorf("seting flag %q value: %w", f.Name, err))
				return
			}
		}
	})

	return errors.Join(bindFlagErr...)
}
