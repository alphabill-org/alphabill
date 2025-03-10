package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	orchestrationsdk "github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	tokenssdk "github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/spf13/cobra"
)

const (
	defaultInitialBillValue   = 1000000000000000000
	defaultDCMoneySupplyValue = 1000000000000000000
)

type (
	shardConfGenerateFlags struct {
		*baseFlags

		NetworkID       uint16
		PartitionID     uint32
		PartitionTypeID uint32
		ShardID         string
		Epoch           uint64
		EpochStart      uint64
		NodeInfoFiles   []string

		MoneyInitialBillOwnerPredicate string
	}
)

func shardConfGenerateCmd(baseFlags *baseFlags) *cobra.Command {
	flags := &shardConfGenerateFlags{baseFlags: baseFlags}
	var cmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate a new shard configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return shardConfGenerate(flags)
		},
	}
	cmd.Flags().Uint16Var(&flags.NetworkID, "network-id", 0, "network identifier")
	cmd.Flags().Uint32Var(&flags.PartitionID, "partition-id", 0, "partition identifier")
	cmd.Flags().Uint32Var(&flags.PartitionTypeID, "partition-type-id", 0, "partition type identifier")
	cmd.Flags().StringVar(&flags.ShardID, "shard-id", "0x80", "the shard id in hex format with 0x prefix")

	cmd.Flags().Uint64Var(&flags.Epoch, "epoch", 0, "epoch assigned to this configuration, must be one greater than the epoch of the previous configuration")
	cmd.Flags().Uint64Var(&flags.EpochStart, "epoch-start", 0, "root round in which this configuration is activated")
	if err := cmd.MarkFlagRequired("epoch-start"); err != nil {
		panic(err)
	}
	cmd.Flags().StringSliceVarP(&flags.NodeInfoFiles, "node-info", "n", []string{}, "path to node info files")
	if err := cmd.MarkFlagRequired("node-info"); err != nil {
		panic(err)
	}
	cmd.Flags().StringVar(&flags.MoneyInitialBillOwnerPredicate, "initial-bill-owner-predicate", "",
		"initial bill owner predicate (money partition only)")

	return cmd
}

func shardConfGenerate(flags *shardConfGenerateFlags) error {
	if err := os.MkdirAll(flags.HomeDir, 0700); err != nil { // -rwx------
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	nodes, err := loadNodeInfoFiles(flags.NodeInfoFiles)
	if err != nil {
		return fmt.Errorf("failed to read node genesis files: %w", err)
	}

	// verify all nodes belong to the same network and partition
	if len(nodes) == 0 {
		return errors.New("at least one node info file must be provided")
	}

	// parse the shardID
	shardID := types.ShardID{}
	if err = shardID.UnmarshalText([]byte(flags.ShardID)); err != nil {
		return fmt.Errorf("failed to parse shard id: %w", err)
	}

	shardConf := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       types.NetworkID(flags.NetworkID),
		PartitionID:     types.PartitionID(flags.PartitionID),
		PartitionTypeID: types.PartitionTypeID(flags.PartitionTypeID),
		ShardID:         shardID,
		Epoch:           flags.Epoch,
		EpochStart:      flags.EpochStart,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2500 * time.Millisecond,
		Validators:      nodes,
		PartitionParams: defaultPartitionParams(types.PartitionTypeID(flags.PartitionTypeID), flags),
	}

	if err = shardConf.IsValid(); err != nil {
		return fmt.Errorf("invalid shard configuration: %w", err)
	}

	fileName := fmt.Sprintf("shard-conf-%d_%d.json", flags.PartitionID, flags.Epoch)
	outputFile := filepath.Join(flags.HomeDir, fileName)
	if err = util.WriteJsonFile(outputFile, shardConf); err != nil {
		return fmt.Errorf("failed to save '%s': %w", outputFile, err)
	}
	return nil
}

func defaultPartitionParams(partitionTypeID types.PartitionTypeID, flags *shardConfGenerateFlags) map[string]string {
	partitionParams := make(map[string]string, 1)
	alwaysTruePredicate := string(hex.Encode(templates.AlwaysTrueBytes()))

	switch partitionTypeID {
	case moneysdk.PartitionTypeID:
		op := flags.MoneyInitialBillOwnerPredicate
		if op == "" {
			op = alwaysTruePredicate
		}
		partitionParams[moneyInitialBillOwnerPredicate] = op
		partitionParams[moneyInitialBillValue]          = strconv.FormatUint(defaultInitialBillValue, 10)
		partitionParams[moneyDCMoneySupplyValue]        = strconv.FormatUint(defaultDCMoneySupplyValue, 10)
	case evmsdk.PartitionTypeID:
		partitionParams[evmGasUnitPrice]  = strconv.FormatUint(evm.DefaultGasPrice, 10)
		partitionParams[evmBlockGasLimit] = strconv.FormatUint(evm.DefaultBlockGasLimit, 10)
	case orchestrationsdk.PartitionTypeID:
		partitionParams[orchestrationOwnerPredicate] = alwaysTruePredicate
	case tokenssdk.PartitionTypeID:
		partitionParams[tokensAdminOwnerPredicate] = ""
		partitionParams[tokensFeelessMode]         = "false"
	}

	return partitionParams
}
