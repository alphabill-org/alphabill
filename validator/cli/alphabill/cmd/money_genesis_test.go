package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	money2 "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/validator/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

const alphabillDir = "ab"
const moneyGenesisDir = "money"

func TestMoneyGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestDir(t, alphabillDir)
	cmd := New(logger.LoggerBuilder(t))
	args := "money-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())

	s := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", s))
}

func TestMoneyGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New(logger.LoggerBuilder(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
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

	cmd := New(logger.LoggerBuilder(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
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

	cmd := New(logger.LoggerBuilder(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
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

	cmd := New(logger.LoggerBuilder(t))
	args := "money-genesis --gen-keys -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
}

func TestMoneyGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, moneyGenesisDir, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, moneyGenesisDir, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, moneyGenesisDir, "n1", moneyGenesisFileName)

	cmd := New(logger.LoggerBuilder(t))
	args := "money-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.EqualValues(t, []byte{1, 1, 1, 1}, pn.BlockCertificationRequest.SystemIdentifier)
}

func TestMoneyGenesis_DefaultParamsExist(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New(logger.LoggerBuilder(t))
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	gf := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	pg, err := util.ReadJsonFile(gf, &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.NotNil(t, pg)

	params := &genesis.MoneyPartitionParams{}
	err = cbor.Unmarshal(pg.Params, params)
	require.NoError(t, err)

	require.EqualValues(t, defaultInitialBillValue, params.InitialBillValue)
	require.EqualValues(t, defaultDCMoneySupplyValue, params.DcMoneySupplyValue)
	require.Len(t, params.SystemDescriptionRecords, 1)
	require.Equal(t, defaultMoneySDR, params.SystemDescriptionRecords[0])
}

func TestMoneyGenesis_ParamsCanBeChanged(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	sdr := &genesis.SystemDescriptionRecord{
		SystemIdentifier: money2.DefaultSystemIdentifier,
		T2Timeout:        10000,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         money2.NewBillID(nil, []byte{2}),
			OwnerPredicate: templates.AlwaysFalseBytes(),
		},
	}
	sdrFile, err := createSDRFile(homeDir, sdr)
	require.NoError(t, err)

	cmd := New(logger.LoggerBuilder(t))
	args := fmt.Sprintf("money-genesis --home %s -g --initial-bill-value %d --dc-money-supply-value %d --system-description-record-files %s", homeDir, 1, 2, sdrFile)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	gf := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	pg, err := util.ReadJsonFile(gf, &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.NotNil(t, pg)

	params := &genesis.MoneyPartitionParams{}
	err = cbor.Unmarshal(pg.Params, params)
	require.NoError(t, err)

	require.EqualValues(t, 1, params.InitialBillValue)
	require.EqualValues(t, 2, params.DcMoneySupplyValue)
	require.Equal(t, sdr, params.SystemDescriptionRecords[0])
}

func createSDRFile(dir string, sdr *genesis.SystemDescriptionRecord) (string, error) {
	filePath := path.Join(dir, "money-sdr.json")
	err := util.WriteJsonFile(filePath, sdr)
	if err != nil {
		return "", err
	}
	return filePath, nil
}
