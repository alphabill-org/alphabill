package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
)

var defaultMoneyShardConf = &types.PartitionDescriptionRecord{
	Version:         1,
	NetworkID:       types.NetworkLocal,
	PartitionID:     moneysdk.DefaultPartitionID,
	PartitionTypeID: moneysdk.PartitionTypeID,
	TypeIDLen:       8,
	UnitIDLen:       256,
	T2Timeout:       2500 * time.Millisecond,
	FeeCreditBill: &types.FeeCreditBill{
		UnitID:         append(make(types.UnitID, 31), 2, moneysdk.BillUnitType),
		OwnerPredicate: templates.AlwaysTrueBytes(),
	},
	PartitionParams: map[string]string{
		moneyInitialBillValue: "500",
	},
}

func Test_MoneyGenesis(t *testing.T) {
	shardConf := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       types.NetworkLocal,
		PartitionID:     moneysdk.DefaultPartitionID,
		PartitionTypeID: moneysdk.PartitionTypeID,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       10 * time.Second,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         append(make(types.UnitID, 31), 2, moneysdk.BillUnitType),
			OwnerPredicate: templates.AlwaysFalseBytes(),
		},
		PartitionParams: map[string]string{
			moneyInitialBillValue:          "1",
			moneyInitialBillOwnerPredicate: fmt.Sprintf("0x%x", templates.AlwaysTrueBytes()),
			moneyDCMoneySupplyValue:        "2",
		},
	}

	t.Run("GenesisStateExists", func(t *testing.T) {
		homeDir := writeShardConf(t, shardConf)
		genesisStatePath := filepath.Join(homeDir, StateFileName)
		_, err := os.Create(genesisStatePath)
		require.NoError(t, err)

		cmd := New(testobserve.NewFactory(t))
		cmd.baseCmd.SetArgs([]string{"shard-conf", "genesis", "--home", homeDir})
		err = cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("state file %q already exists", genesisStatePath))
	})

	t.Run("MoneyGenesisState", func(t *testing.T) {
		homeDir := writeShardConf(t, shardConf)

		cmd := New(testobserve.NewFactory(t))
		cmd.baseCmd.SetArgs([]string{"shard-conf", "genesis", "--home", homeDir})
		require.NoError(t, cmd.Execute(context.Background()))
	})

	t.Run("TokensGenesisState", func(t *testing.T) {
		adminOwnerPredicate := "830041025820f34a250bf4f2d3a432a43381cecc4ab071224d9ceccb6277b5779b937f59055f"
		shardConf := &types.PartitionDescriptionRecord{
			Version:         1,
			NetworkID:       5,
			PartitionID:     tokens.DefaultPartitionID,
			PartitionTypeID: tokens.PartitionTypeID,
			TypeIDLen:       8,
			UnitIDLen:       256,
			T2Timeout:       3 * time.Second,
			PartitionParams: map[string]string{
				tokensAdminOwnerPredicate: adminOwnerPredicate,
				tokensFeelessMode:         "true",
			},
		}
		homeDir := writeShardConf(t, shardConf)

		cmd := New(testobserve.NewFactory(t))
		cmd.baseCmd.SetArgs([]string{"shard-conf", "genesis", "--home", homeDir})
		require.NoError(t, cmd.Execute(context.Background()))

		// TODO: test this under shard_conf_generate_test.go
		// nodeGenesisFile := filepath.Join(homeDir, genesisStateFileName)
		// pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{Version: 1})
		// require.NoError(t, err)

		// params, err := genesis.ParseTokensPartitionParams(&pn.PartitionDescriptionRecord)
		// require.NoError(t, err)

		// require.NotNil(t, params)
		// require.Equal(t, adminOwnerPredicate, hex.EncodeToString(params.AdminOwnerPredicate))
		// require.True(t, params.FeelessMode)
	})

	t.Run("EvmGenesisState", func(t *testing.T) {
		shardConf := types.PartitionDescriptionRecord{
			Version:         1,
			NetworkID:       5,
			PartitionID:     evm.DefaultPartitionID,
			PartitionTypeID: evm.PartitionTypeID,
			TypeIDLen:       8,
			UnitIDLen:       256,
			T2Timeout:       2500 * time.Millisecond,
			PartitionParams: map[string]string{
				evmGasUnitPrice:  "9223372036854775808",
				evmBlockGasLimit: "100000",
			},
		}

		homeDir := writeShardConf(t, &shardConf)
		cmd := New(testobserve.NewFactory(t))
		cmd.baseCmd.SetArgs([]string{"shard-conf", "genesis", "--home", homeDir})
		require.NoError(t, cmd.Execute(context.Background()))
	})

	t.Run("OrchestrationGenesisState", func(t *testing.T) {
		const ownerPredicate = "830041025820F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0"

		shardConf := &types.PartitionDescriptionRecord{
			Version:         1,
			NetworkID:       5,
			PartitionID:     orchestration.DefaultPartitionID,
			PartitionTypeID: orchestration.PartitionTypeID,
			TypeIDLen:       8,
			UnitIDLen:       256,
			T2Timeout:       2500 * time.Millisecond,
			PartitionParams: map[string]string{
				orchestrationOwnerPredicate: ownerPredicate,
			},
		}
		homeDir := writeShardConf(t, shardConf)
		cmd := New(testobserve.NewFactory(t))
		cmd.baseCmd.SetArgs([]string{"shard-conf", "genesis", "--home", homeDir})
		require.NoError(t, cmd.Execute(context.Background()))

		// TOOD: shard_conf_generate_test.go
		// params, err := genesis.ParseOrchestrationPartitionParams(&pn.PartitionDescriptionRecord)
		// require.NoError(t, err)
		// expectedOwnerPredicate, err := hex.DecodeString(ownerPredicate)
		// require.NoError(t, err)
		// require.EqualValues(t, expectedOwnerPredicate, params.OwnerPredicate)
	})
}

func writeShardConf(t *testing.T, shardConf *types.PartitionDescriptionRecord) string {
	homeDir := t.TempDir()
	filePath := filepath.Join(homeDir, shardConfFileName)
	require.NoError(t, util.WriteJsonFile(filePath, shardConf))
	return homeDir
}
