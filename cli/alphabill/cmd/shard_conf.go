package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/spf13/cobra"
)

const shardConfFileName = "shard-conf.json"

type (
	shardConfFlags struct {
		ShardConfFile string
	}
)

func newShardConfCmd(baseConfig *baseFlags) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "shard-conf",
		Short: "Tools to work with shard configuration files",
	}
	cmd.AddCommand(shardConfGenerateCmd(baseConfig))
	cmd.AddCommand(shardConfGenesisCmd(baseConfig))
	return cmd
}

func (f *shardConfFlags) addShardConfFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.ShardConfFile, "shard-conf", "s", "",
		fmt.Sprintf("path to shard conf (default: %s)", filepath.Join("$AB_HOME", shardConfFileName)))
}

func (f *shardConfFlags) shardConfPath(baseFlags *baseFlags) string {
	return baseFlags.pathWithDefault(f.ShardConfFile, shardConfFileName)
}

func (f *shardConfFlags) loadShardConf(baseFlags *baseFlags) (ret *types.PartitionDescriptionRecord, err error) {
	return ret, baseFlags.loadConf(f.ShardConfFile, shardConfFileName, &ret)
}
