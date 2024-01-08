package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

const alphabillDir = "ab"
const moneyGenesisDir = "money"

func TestMoneyGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := t.TempDir()
	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())

	s := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", s))
}

func TestMoneyGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName))
	require.FileExists(t, filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName))
}

func TestMoneyGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis file %q already exists", nodeGenesisFile))
	require.NoFileExists(t, filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName))
}

func TestMoneyGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)
	kf := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestMoneyGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(filepath.Join(homeDir, alphabillDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir, "n1"), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, moneyGenesisDir, "n1", moneyGenesisFileName)
	nodeGenesisStateFile := filepath.Join(homeDir, moneyGenesisDir, "n1", moneyGenesisStateFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
	require.FileExists(t, nodeGenesisStateFile)
}

func TestMoneyGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, moneyGenesisDir, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, moneyGenesisDir, "n1", moneyGenesisFileName)

	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 4030201" + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.EqualValues(t, 4030201, pn.BlockCertificationRequest.SystemIdentifier)
}

func TestMoneyGenesis_DefaultParamsExist(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.Execute(context.Background())
	require.NoError(t, err)

	gf := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	pg, err := util.ReadJsonFile(gf, &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.NotNil(t, pg)

	params := &genesis.MoneyPartitionParams{}
	err = cbor.Unmarshal(pg.Params, params)
	require.NoError(t, err)

	require.Len(t, params.SystemDescriptionRecords, 1)
	require.Equal(t, defaultMoneySDR, params.SystemDescriptionRecords[0])
}

func TestMoneyGenesis_ParamsCanBeChanged(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	sdr := &genesis.SystemDescriptionRecord{
		SystemIdentifier: money.DefaultSystemIdentifier,
		T2Timeout:        10000,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         money.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysFalseBytes(),
		},
	}
	sdrFile, err := createSDRFile(homeDir, sdr)
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := fmt.Sprintf("money-genesis --home %s -g --initial-bill-value %d --dc-money-supply-value %d --system-description-record-files %s", homeDir, 1, 2, sdrFile)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.NoError(t, err)

	gf := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	pg, err := util.ReadJsonFile(gf, &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.NotNil(t, pg)

	params := &genesis.MoneyPartitionParams{}
	err = cbor.Unmarshal(pg.Params, params)
	require.NoError(t, err)

	require.Equal(t, sdr, params.SystemDescriptionRecords[0])
}

func TestMoneyGenesis_InvalidFeeCreditBill_SameAsInitialBill(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	sdr := &genesis.SystemDescriptionRecord{
		SystemIdentifier: money.DefaultSystemIdentifier,
		T2Timeout:        10000,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         defaultInitialBillID,
			OwnerPredicate: templates.AlwaysFalseBytes(),
		},
	}
	sdrFile, err := createSDRFile(filepath.Join(homeDir, moneyGenesisDir), sdr)
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis -g --home " + homeDir + " --system-description-record-files " + sdrFile
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, "fee credit bill ID may not be equal to")
}

func TestMoneyGenesis_InvalidFeeCreditBill_SameAsDCBill(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	sdr := &genesis.SystemDescriptionRecord{
		SystemIdentifier: money.DefaultSystemIdentifier,
		T2Timeout:        10000,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         money.DustCollectorMoneySupplyID,
			OwnerPredicate: templates.AlwaysFalseBytes(),
		},
	}
	sdrFile, err := createSDRFile(filepath.Join(homeDir, moneyGenesisDir), sdr)
	require.NoError(t, err)

	cmd := New(testobserve.NewFactory(t))
	args := "money-genesis -g --home " + homeDir + " --system-description-record-files " + sdrFile
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.Execute(context.Background())
	require.ErrorContains(t, err, "fee credit bill ID may not be equal to")
}

func Test_moneyGenesisConfig_getSDRFiles(t *testing.T) {
	testDir := t.TempDir()

	t.Run("parse file", func(t *testing.T) {
		// setup-testab.sh creates the SDR files with hardcoded content - refactor
		// so that these files can be tested here?
		fileName := filepath.Join(testDir, "sdr.json")
		err := os.WriteFile(fileName, []byte(`{"system_identifier": 1234567890, "t2timeout": 2500, "fee_credit_bill": {"unit_id": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQ=", "owner_predicate": "U3aoAU8B9SAiu0UEB9kvE78cUxKKZ2vPMEgY6fQaXvTr6unA1rCHaawB"}}`), 0666)
		require.NoError(t, err)

		cfg := moneyGenesisConfig{SDRFiles: []string{fileName}}
		sdrs, err := cfg.getSDRs()
		require.NoError(t, err)
		require.Len(t, sdrs, 1)
		require.EqualValues(t, 1234567890, sdrs[0].SystemIdentifier)
	})

	t.Run("defaultMoneySDR", func(t *testing.T) {
		// serialize and deserialize default money SDR
		fileName, err := createSDRFile(testDir, defaultMoneySDR)
		require.NoError(t, err)
		cfg := moneyGenesisConfig{SDRFiles: []string{fileName}}
		sdrs, err := cfg.getSDRs()
		require.NoError(t, err)
		require.Len(t, sdrs, 1)
		require.Equal(t, defaultMoneySDR, sdrs[0])
	})

	t.Run("if no files default will be returned", func(t *testing.T) {
		cfg := moneyGenesisConfig{SDRFiles: nil} // no files provided!
		sdrs, err := cfg.getSDRs()
		require.NoError(t, err)
		require.Len(t, sdrs, 1)
		require.Equal(t, defaultMoneySDR, sdrs[0])
	})
}

func createSDRFile(dir string, sdr *genesis.SystemDescriptionRecord) (string, error) {
	filePath := filepath.Join(dir, "money-sdr.json")
	if err := util.WriteJsonFile(filePath, sdr); err != nil {
		return "", err
	}
	return filePath, nil
}
