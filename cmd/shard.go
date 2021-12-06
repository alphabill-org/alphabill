package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

type (
	shardConfiguration struct {
		Root   *rootConfiguration
		Server *serverConfiguration
		// The value of initial bill in AlphaBills.
		InitialBillValue uint32 `validate:"gte=0"`
	}
	// shardRunnable is the function that is run after configuration is loaded.
	shardRunnable func(shardConfig *shardConfiguration) error
)

const (
	defaultInitialBillValue = 1000000
)

// newShardCmd creates a new cobra command for the shard component.
//
// shardRunFunc - set the function to override the default behaviour. Meant for tests.
func newShardCmd(rootConfig *rootConfiguration, shardRunFunc shardRunnable) *cobra.Command {
	config := &shardConfiguration{
		Root:   rootConfig,
		Server: &serverConfiguration{},
	}
	// shardCmd represents the shard command
	var shardCmd = &cobra.Command{
		Use:   "shard",
		Short: "Starts a shard node",
		Long:  `Shard a shard component, binding to the network address provided by configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if shardRunFunc != nil {
				return shardRunFunc(config)
			}
			return defaultShardRunFunc(config)
		},
	}

	shardCmd.Flags().Uint32Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value for new shard")
	addServerConfigurationFlags(shardCmd, config.Server)

	return shardCmd
}

func defaultShardRunFunc(cfg *shardConfiguration) error {
	// TODO temporary runnable
	fmt.Println("shard called")
	fmt.Println("cfg file:", cfg.Root.CfgFile)
	fmt.Println("home dir:", cfg.Root.HomeDir)
	fmt.Println("bill val:", cfg.InitialBillValue)
	return nil
}
