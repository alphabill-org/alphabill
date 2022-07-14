package cmd

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

const alphabillDir = "ab"
const moneyGenesisDir = "money"

func TestMoneyGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestDir(t, alphabillDir)
	cmd := New()
	args := "money-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())

	s := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	require.ErrorContains(t, err, fmt.Sprintf("failed to load keys %s", s))
}

func TestMoneyGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New()
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	gf := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	require.FileExists(t, kf)
	require.FileExists(t, gf)
}

func TestMoneyGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New()
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	require.NoFileExists(t, kf)
}

func TestMoneyGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)
	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New()
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestMoneyGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, alphabillDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(homeDir, moneyGenesisDir, "n1"), 0700)
	require.NoError(t, err)

	kf := path.Join(homeDir, moneyGenesisDir, defaultKeysFileName)

	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, "n1", nodeGenesisFileName)

	cmd := New()
	args := "money-genesis --gen-keys -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestMoneyGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	err := os.MkdirAll(path.Join(homeDir, moneyGenesisDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(path.Join(homeDir, moneyGenesisDir, "n1"), 0700)
	require.NoError(t, err)

	kf := path.Join(homeDir, moneyGenesisDir, "n1", defaultKeysFileName)
	nodeGenesisFile := path.Join(homeDir, moneyGenesisDir, "n1", nodeGenesisFileName)

	cmd := New()
	args := "money-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.Equal(t, []byte{1, 1, 1, 1}, pn.BlockCertificationRequest.SystemIdentifier)
}

func TestMoneyGenesis_DefaultParamsExist(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New()
	args := "money-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	gf := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	pg, err := util.ReadJsonFile(gf, &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.NotNil(t, pg)

	params := &genesis.MoneyPartitionParams{}
	err = pg.Params.UnmarshalTo(params)
	require.NoError(t, err)

	require.EqualValues(t, defaultInitialBillValue, params.InitialBillValue)
	require.EqualValues(t, defaultDCMoneySupplyValue, params.DcMoneySupplyValue)
}

func TestMoneyGenesis_ParamsCanBeChanged(t *testing.T) {
	homeDir := setupTestHomeDir(t, alphabillDir)
	cmd := New()
	args := fmt.Sprintf("money-genesis --home %s -g --initial-bill-value %d --dc-money-supply-value %d", homeDir, 1, 2)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	gf := path.Join(homeDir, moneyGenesisDir, nodeGenesisFileName)
	pg, err := util.ReadJsonFile(gf, &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.NotNil(t, pg)

	params := &genesis.MoneyPartitionParams{}
	err = pg.Params.UnmarshalTo(params)
	require.NoError(t, err)

	require.EqualValues(t, 1, params.InitialBillValue)
	require.EqualValues(t, 2, params.DcMoneySupplyValue)
}
