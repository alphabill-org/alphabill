package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
)

var defaultMoneyPDR = &types.PartitionDescriptionRecord{
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
		"initialBillValue": "500",
	},
}

func Test_MoneyGenesis(t *testing.T) {
	pdr := &types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   types.NetworkLocal,
		PartitionID: moneysdk.DefaultPartitionID,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   10 * time.Second,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         append(make(types.UnitID, 31), 2, moneysdk.BillUnitType),
			OwnerPredicate: templates.AlwaysFalseBytes(),
		},
		PartitionParams: map[string]string{
			"initialBillValue": "1",
			"initialBillOwnerPredicate": fmt.Sprintf("%x", templates.AlwaysTrueBytes()),
			"dcMoneySupplyValue": "2",
		},
	}
	homeDir := createPDRFile(t, pdr) // TODO: should return pdr file location
	pdrArgument := " --partition-description " + filepath.Join(homeDir, shardConfFileName)

	t.Run("KeyFileNotFound", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "genesis --home " + homeDir
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		s := filepath.Join(homeDir, keyConfFileName)
		require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", s))
	})

	t.Run("ForceKeyGeneration", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, filepath.Join(homeDir, keyConfFileName))
		require.FileExists(t, filepath.Join(homeDir, stateFileName))
	})

	t.Run("GenesisStateExists", func(t *testing.T) {
		homeDir := t.TempDir()
		genesisStatePath := filepath.Join(homeDir, stateFileName)
		_, err := os.Create(genesisStatePath)
		require.NoError(t, err)

		cmd := New(testobserve.NewFactory(t))
		args := "genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err = cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("genesis state %q already exists", genesisStatePath))
		require.NoFileExists(t, filepath.Join(homeDir, keyConfFileName))
	})

	t.Run("LoadExistingKeys", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir), 0700))
		kf := filepath.Join(homeDir, keyConfFileName)
		nodeGenesisFile := filepath.Join(homeDir, stateFileName)
		nodeKeys, err := generateKeys()
		require.NoError(t, err)
		require.NoError(t, nodeKeys.WriteTo(kf))

		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, kf)
		require.FileExists(t, nodeGenesisFile)
	})

	// t.Run("WritesGenesisToSpecifiedOutputLocation", func(t *testing.T) {
	// 	homeDir := t.TempDir()
	// 	require.NoError(t, os.MkdirAll(filepath.Join(homeDir, moneyPartitionDir, "n1"), 0700))

	// 	nodeGenesisFile := filepath.Join(homeDir, moneyPartitionDir, "n1", moneyGenesisFileName)
	// 	nodeGenesisStateFile := filepath.Join(homeDir, moneyPartitionDir, "n1", moneyGenesisStateFileName)

	// 	cmd := New(testobserve.NewFactory(t))
	// 	args := "money-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir + pdrArgument
	// 	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	// 	require.NoError(t, cmd.Execute(context.Background()))

	// 	require.FileExists(t, filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName))
	// 	require.FileExists(t, nodeGenesisFile)
	// 	require.FileExists(t, nodeGenesisStateFile)
	// })

	// // TODO: not changing params any more, rename or move
	// t.Run("ParamsCanBeChanged", func(t *testing.T) {
	// 	homeDir := t.TempDir()
	// 	// pdr.FeeCreditBill.UnitID, err = pdr.ComposeUnitID(types.ShardID{}, moneysdk.BillUnitType, moneyid.Random)
	// 	// require.NoError(t, err)

	// 	cmd := New(testobserve.NewFactory(t))
	// 	args := fmt.Sprintf("money-genesis --home %s -g %s", homeDir, pdrArgument)
	// 	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	// 	require.NoError(t, cmd.Execute(context.Background()))

	// 	pn, err := util.ReadJsonFile(filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName), &genesis.PartitionNode{})
	// 	require.NoError(t, err)
	// 	require.NotNil(t, pn)

	// 	params, err := genesis.ParseMoneyPartitionParams(&pn.PartitionDescriptionRecord)
	// 	require.NoError(t, err)
	// 	require.EqualValues(t, 1, params.InitialBillValue)
	// 	require.Equal(t, ownerPredicate, fmt.Sprintf("%X", params.InitialBillOwnerPredicate))

	// 	stateFile, err := os.Open(filepath.Join(homeDir, moneyPartitionDir, moneyGenesisStateFileName))
	// 	require.NoError(t, err)
	// 	s, err := state.NewRecoveredState(stateFile, func(ui types.UnitID) (types.UnitData, error) {
	// 		return moneysdk.NewUnitData(ui, pdr)
	// 	})
	// 	require.NoError(t, err)
	// 	unit, err := s.GetUnit(moneyPartitionInitialBillID, false)
	// 	require.NoError(t, err)
	// 	require.Equal(t, fmt.Sprintf("%X", unit.Data().Owner()), ownerPredicate)
	// })

	// TODO: test generating genesis state for different partition types
	// t.Run("TokensPartitionParams", func(t *testing.T) {
	// 	adminOwnerPredicate := "830041025820f34a250bf4f2d3a432a43381cecc4ab071224d9ceccb6277b5779b937f59055f"
	// 	pdr2 := types.PartitionDescriptionRecord{
	// 		Version:     1,
	// 		NetworkID:   5,
	// 		PartitionID: 2,
	// 		TypeIDLen:   8,
	// 		UnitIDLen:   256,
	// 		T2Timeout:   3 * time.Second,
	// 	}

	// 	pdr2.PartitionParams = map[string]string{
	// 		"adminOwnerPredicate": adminOwnerPredicate,
	// 		"feeless-mode": "true",
	// 	}
	// 	homeDir := createPDRFile(t, &pdr2)

	// 	cmd := New(testobserve.NewFactory(t))
	// 	args := fmt.Sprintf("genesis -g --home %s", homeDir)
	// 	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	// 	require.NoError(t, cmd.Execute(context.Background()))

	// 	nodeGenesisFile := filepath.Join(homeDir, genesisStateFileName)
	// 	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{Version: 1})
	// 	require.NoError(t, err)

	// 	params, err := genesis.ParseTokensPartitionParams(&pn.PartitionDescriptionRecord)
	// 	require.NoError(t, err)

	// 	require.NotNil(t, params)
	// 	require.Equal(t, adminOwnerPredicate, hex.EncodeToString(params.AdminOwnerPredicate))
	// 	require.True(t, params.FeelessMode)
	// })

	t.Run("EvmPartitionParams", func(t *testing.T) {
		// Create a new PDR file with invalid GasUnitPrice
		pdr2 := types.PartitionDescriptionRecord{
			Version:         1,
			NetworkID:       5,
			PartitionID:     evm.DefaultPartitionID,
			TypeIDLen:       8,
			UnitIDLen:       256,
			T2Timeout:       2500 * time.Millisecond,
			PartitionParams: map[string]string{
				"gasUnitPrice":  "1",
				"blockGasLimit": "999",
			},
		}

		pdr2.PartitionID = 999
		pdr2.PartitionParams = map[string]string{
			"gasUnitPrice":  "9223372036854775808",
			"blockGasLimit": "100000",
		}
		homeDir := createPDRFile(t, &pdr2)

		kf := filepath.Join(homeDir, stateFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "genesis -g -k " + kf + " --home " + homeDir
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, "invalid gasUnitPrice, max value is")
	})

	// t.Run("OrchestrationPartitionParams", func(t *testing.T) {
	// 	const ownerPredicate = "830041025820F52022BB450407D92F13BF1C53128A676BCF304818E9F41A5EF4EBEAE9C0D6B0"

	// 	pdr := types.PartitionDescriptionRecord{
	// 		Version:     1,
	// 		NetworkID:   5,
	// 		PartitionID: 55,
	// 		TypeIDLen:   4,
	// 		UnitIDLen:   300,
	// 		T2Timeout:   10 * time.Second,
	// 		PartitionParams: map[string]string{
	// 			"ownerPredicate":  ownerPredicate, // TODO: const
	// 		},
	// 	}
	// 	homeDir := createPDRFile(t, &pdr)

	// 	cmd := New(testobserve.NewFactory(t))
	// 	args := "orchestration-genesis -g --home " + homeDir
	// 	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	// 	require.NoError(t, cmd.Execute(context.Background()))

	// 	params, err := genesis.ParseOrchestrationPartitionParams(&pn.PartitionDescriptionRecord)
	// 	require.NoError(t, err)

	// 	expectedOwnerPredicate, err := hex.DecodeString(ownerPredicate)
	// 	require.NoError(t, err)
	// 	require.EqualValues(t, expectedOwnerPredicate, params.OwnerPredicate)
	// })
}

func createPDRFile(t *testing.T, pdr *types.PartitionDescriptionRecord) string {
	homeDir := t.TempDir()
	filePath := filepath.Join(homeDir, shardConfFileName)
	require.NoError(t, util.WriteJsonFile(filePath, pdr))
	return homeDir
}
