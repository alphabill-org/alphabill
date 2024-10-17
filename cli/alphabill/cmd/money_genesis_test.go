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
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/money"
)

func Test_MoneyGenesis(t *testing.T) {
	pdrFilename, err := createPDRFile(t.TempDir(), defaultMoneyPDR)
	require.NoError(t, err)
	pdrArgument := " --partition-description " + pdrFilename

	t.Run("KeyFileNotFound", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		s := filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName)
		require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", s))
	})

	t.Run("ForceKeyGeneration", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName))
		require.FileExists(t, filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName))
	})

	t.Run("DefaultNodeGenesisExists", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, moneyPartitionDir), 0700))
		nodeGenesisFile := filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName)
		require.NoError(t, util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"}))

		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err = cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("node genesis file %q already exists", nodeGenesisFile))
		require.NoFileExists(t, filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName))
	})

	t.Run("LoadExistingKeys", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, moneyPartitionDir), 0700))
		kf := filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName)
		nodeGenesisFile := filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName)
		nodeKeys, err := GenerateKeys()
		require.NoError(t, err)
		require.NoError(t, nodeKeys.WriteTo(kf))

		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, kf)
		require.FileExists(t, nodeGenesisFile)
	})

	t.Run("WritesGenesisToSpecifiedOutputLocation", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, moneyPartitionDir, "n1"), 0700))

		nodeGenesisFile := filepath.Join(homeDir, moneyPartitionDir, "n1", moneyGenesisFileName)
		nodeGenesisStateFile := filepath.Join(homeDir, moneyPartitionDir, "n1", moneyGenesisStateFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName))
		require.FileExists(t, nodeGenesisFile)
		require.FileExists(t, nodeGenesisStateFile)
	})

	t.Run("DefaultParamsExist", func(t *testing.T) {
		homeDir := t.TempDir()
		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis --gen-keys --home " + homeDir + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		pg, err := util.ReadJsonFile(filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName), &genesis.PartitionGenesis{})
		require.NoError(t, err)
		require.NotNil(t, pg)

		params := &genesis.MoneyPartitionParams{}
		require.NoError(t, types.Cbor.Unmarshal(pg.Params, params))
		require.Len(t, params.Partitions, 1)
		require.Equal(t, defaultMoneyPDR, params.Partitions[0])
	})

	t.Run("ParamsCanBeChanged", func(t *testing.T) {
		homeDir := t.TempDir()
		sdr := &types.PartitionDescriptionRecord{
			NetworkIdentifier: 5,
			SystemIdentifier:  moneysdk.DefaultSystemID,
			TypeIdLen:         4,
			UnitIdLen:         300,
			T2Timeout:         10 * time.Second,
			FeeCreditBill: &types.FeeCreditBill{
				UnitID:         moneysdk.NewBillID(nil, []byte{2}),
				OwnerPredicate: templates.AlwaysFalseBytes(),
			},
		}
		sdrFile, err := createPDRFile(homeDir, sdr)
		require.NoError(t, err)

		cmd := New(testobserve.NewFactory(t))
		args := fmt.Sprintf("money-genesis --home %s -g --initial-bill-value %d --initial-bill-owner-predicate %s"+
			" --dc-money-supply-value %d --system-description-record-files %s %s", homeDir, 1, ownerPredicate, 2, sdrFile, pdrArgument)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		pg, err := util.ReadJsonFile(filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName), &genesis.PartitionGenesis{})
		require.NoError(t, err)
		require.NotNil(t, pg)

		stateFile, err := os.Open(filepath.Join(homeDir, moneyPartitionDir, moneyGenesisStateFileName))
		require.NoError(t, err)
		s, err := state.NewRecoveredState(stateFile, moneysdk.NewUnitData)
		require.NoError(t, err)
		unit, err := s.GetUnit(defaultInitialBillID, false)
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("%X", unit.Data().Owner()), ownerPredicate)

		params := &genesis.MoneyPartitionParams{}
		require.NoError(t, types.Cbor.Unmarshal(pg.Params, params))
		require.Equal(t, sdr, params.Partitions[0])
	})

	t.Run("InvalidFeeCreditBill_SameAsInitialBill", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, moneyPartitionDir), 0700))

		sdr := &types.PartitionDescriptionRecord{
			NetworkIdentifier: 5,
			SystemIdentifier:  moneysdk.DefaultSystemID,
			T2Timeout:         10 * time.Second,
			FeeCreditBill: &types.FeeCreditBill{
				UnitID:         defaultInitialBillID,
				OwnerPredicate: templates.AlwaysFalseBytes(),
			},
		}
		sdrFile, err := createPDRFile(filepath.Join(homeDir, moneyPartitionDir), sdr)
		require.NoError(t, err)

		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis -g --home " + homeDir + " --system-description-record-files " + sdrFile + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err = cmd.Execute(context.Background())
		require.ErrorContains(t, err, "fee credit bill ID may not be equal to")
	})

	t.Run("InvalidFeeCreditBill_SameAsDCBill", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, moneyPartitionDir), 0700))

		sdr := &types.PartitionDescriptionRecord{
			NetworkIdentifier: 5,
			SystemIdentifier:  moneysdk.DefaultSystemID,
			T2Timeout:         10 * time.Second,
			FeeCreditBill: &types.FeeCreditBill{
				UnitID:         money.DustCollectorMoneySupplyID,
				OwnerPredicate: templates.AlwaysFalseBytes(),
			},
		}
		sdrFile, err := createPDRFile(filepath.Join(homeDir, moneyPartitionDir), sdr)
		require.NoError(t, err)

		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis -g --home " + homeDir + " --system-description-record-files " + sdrFile + pdrArgument
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err = cmd.Execute(context.Background())
		require.ErrorContains(t, err, "fee credit bill ID may not be equal to")
	})

	t.Run("partition description is loaded", func(t *testing.T) {
		homeDir := t.TempDir()
		nodeGenesisFile := filepath.Join(homeDir, moneyPartitionDir, evmGenesisFileName)

		sdr := types.PartitionDescriptionRecord{
			NetworkIdentifier: 5,
			SystemIdentifier:  55,
			TypeIdLen:         4,
			UnitIdLen:         300,
			T2Timeout:         10 * time.Second,
			FeeCreditBill: &types.FeeCreditBill{
				UnitID:         moneysdk.NewBillID(nil, []byte{2}),
				OwnerPredicate: templates.AlwaysFalseBytes(),
			},
		}
		sdrFile, err := createPDRFile(homeDir, &sdr)
		require.NoError(t, err)

		cmd := New(testobserve.NewFactory(t))
		args := "money-genesis -g -o " + nodeGenesisFile + " --home " + homeDir + " --partition-description " + sdrFile
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
		require.NoError(t, err)
		require.EqualValues(t, sdr, pn.PartitionDescription)
		require.EqualValues(t, sdr.SystemIdentifier, pn.BlockCertificationRequest.Partition)
	})
}

func Test_moneyGenesisConfig_getSDRFiles(t *testing.T) {
	testDir := t.TempDir()

	t.Run("parse file", func(t *testing.T) {
		// setup-testab.sh creates the SDR files with hardcoded content - refactor
		// so that these files can be tested here?
		fileName := filepath.Join(testDir, "sdr.json")
		err := os.WriteFile(fileName, []byte(`{"system_identifier": 1234567890, "t2timeout": 2500, "fee_credit_bill": {"unit_id": "0x00000000000000000000000000000000000000000000000000000000000000010F", "owner_predicate": "0x5376a8014f01f52022bb450407d92f13bf1c53128a676bcf304818e9f41a5ef4ebeae9c0d6b08769ac01"}}`), 0666)
		require.NoError(t, err)

		cfg := moneyGenesisConfig{SDRFiles: []string{fileName}}
		sdrs, err := cfg.getSDRs()
		require.NoError(t, err)
		require.Len(t, sdrs, 1)
		require.EqualValues(t, 1234567890, sdrs[0].SystemIdentifier)
	})

	t.Run("defaultMoneySDR", func(t *testing.T) {
		// serialize and deserialize default money SDR
		fileName, err := createPDRFile(testDir, defaultMoneyPDR)
		require.NoError(t, err)
		cfg := moneyGenesisConfig{SDRFiles: []string{fileName}}
		sdrs, err := cfg.getSDRs()
		require.NoError(t, err)
		require.Len(t, sdrs, 1)
		require.Equal(t, defaultMoneyPDR, sdrs[0])
	})

	t.Run("if no files default will be returned", func(t *testing.T) {
		cfg := moneyGenesisConfig{SDRFiles: nil} // no files provided!
		sdrs, err := cfg.getSDRs()
		require.NoError(t, err)
		require.Len(t, sdrs, 1)
		require.Equal(t, defaultMoneyPDR, sdrs[0])
	})
}

func createPDRFile(dir string, pdr *types.PartitionDescriptionRecord) (string, error) {
	filePath := filepath.Join(dir, fmt.Sprintf("pdr-%d.json", pdr.SystemIdentifier))
	if err := util.WriteJsonFile(filePath, pdr); err != nil {
		return "", err
	}
	return filePath, nil
}
