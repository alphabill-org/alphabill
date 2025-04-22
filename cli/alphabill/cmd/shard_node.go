package cmd

import (
	"github.com/spf13/cobra"
)

// newShardNodeCmd creates a new cobra command for shard node management.
//
// shardNodeRunFn - set the function to override the default behavior. Meant for tests.
func newShardNodeCmd(baseFlags *baseFlags, shardNodeRunFn nodeRunnable) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "shard-node",
		Short: "Tools to run a shard node",
	}
	cmd.AddCommand(shardNodeInitCmd(baseFlags))
	cmd.AddCommand(shardNodeRunCmd(baseFlags, shardNodeRunFn))
	return cmd
}
