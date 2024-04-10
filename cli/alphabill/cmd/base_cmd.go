package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type (
	alphabillApp struct {
		baseCmd    *cobra.Command
		baseConfig *baseConfiguration
	}

	Factory interface {
		Logger(cfg *logger.LogConfiguration) (*slog.Logger, error)
		Observability(metrics, traces string) (observability.MeterAndTracer, error)
	}
)

// New creates a new Alphabill application
func New(obsF Factory, opts ...interface{}) *alphabillApp {
	baseCmd, baseConfig := newBaseCmd(obsF)
	app := &alphabillApp{baseCmd: baseCmd, baseConfig: baseConfig}
	app.addSubcommands(opts)
	return app
}

// Execute runs the application
func (a *alphabillApp) Execute(ctx context.Context) (err error) {
	defer func() {
		if a.baseConfig.observe != nil {
			err = errors.Join(err, a.baseConfig.observe.Shutdown())
		}
	}()

	return a.baseCmd.ExecuteContext(ctx)
}

func (a *alphabillApp) addSubcommands(opts []interface{}) {
	a.baseCmd.AddCommand(newMoneyNodeCmd(a.baseConfig, convertOptsToRunnable(opts)))
	a.baseCmd.AddCommand(newMoneyGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newRootGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newRootNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newNodeIdentifierCmd())
	a.baseCmd.AddCommand(newTokensNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newUserTokenGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newEvmNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newEvmGenesisCmd(a.baseConfig))
	a.baseCmd.AddCommand(newOrchestrationNodeCmd(a.baseConfig))
	a.baseCmd.AddCommand(newOrchestrationGenesisCmd(a.baseConfig))
}

func newBaseCmd(obsF Factory) (*cobra.Command, *baseConfiguration) {
	config := &baseConfiguration{}
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
			if err := initializeConfig(cmd, config, obsF); err != nil {
				return fmt.Errorf("failed to initialize configuration: %w", err)
			}
			return nil
		},
	}
	config.addConfigurationFlags(baseCmd)

	return baseCmd, config
}

func initializeConfig(cmd *cobra.Command, config *baseConfiguration, obsF Factory) error {
	var errs []error

	if err := config.initializeConfig(cmd); err != nil {
		errs = append(errs, fmt.Errorf("reading configuration: %w", err))
	}

	logger, err := config.initLogger(cmd, obsF.Logger)
	if err != nil {
		errs = append(errs, fmt.Errorf("initializing logger: %w", err))
	}

	metrics, err := cmd.Flags().GetString(keyMetrics)
	if err != nil {
		errs = append(errs, fmt.Errorf("reading flag %q: %w", keyMetrics, err))
	}
	tracing, err := cmd.Flags().GetString(keyTracing)
	if err != nil {
		errs = append(errs, fmt.Errorf("reading flag %q: %w", keyTracing, err))
	}
	observe, err := obsF.Observability(metrics, tracing)
	if err != nil {
		errs = append(errs, fmt.Errorf("initializing observability: %w", err))
	}
	if observe != nil && logger != nil {
		config.observe = observability.WithLogger(observe, logger)
	}

	return errors.Join(errs...)
}

// initializeConfig reads in config file and ENV variables if set.
func (r *baseConfiguration) initializeConfig(cmd *cobra.Command) error {
	v := viper.New()

	r.initConfigFileLocation()

	if r.configFileExists() {
		v.SetConfigFile(r.CfgFile)
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
