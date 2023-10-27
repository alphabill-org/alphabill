package cmd

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/common/util"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/testutils/logger"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestEvmGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("load keys %s failed", filepath.Join(homeDir, evmDir, defaultKeysFileName)))
}

func TestEvmGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(homeDir, evmDir, defaultKeysFileName))
	require.FileExists(t, filepath.Join(homeDir, evmDir, evmGenesisFileName))
}

func TestEvmGenesis_DefaultNodeGenesisExists(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	err := os.MkdirAll(filepath.Join(homeDir, evmDir), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, evmDir, evmGenesisFileName)
	err = util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{NodeIdentifier: "1"})
	require.NoError(t, err)

	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, fmt.Sprintf("node genesis %s exists", nodeGenesisFile))
	require.NoFileExists(t, filepath.Join(homeDir, evmDir, defaultKeysFileName))
}

func TestEvmGenesis_LoadExistingKeys(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	err := os.MkdirAll(filepath.Join(homeDir, evmDir), 0700)
	require.NoError(t, err)
	kf := filepath.Join(homeDir, evmDir, defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, evmDir, evmGenesisFileName)
	nodeKeys, err := GenerateKeys()
	require.NoError(t, err)
	err = nodeKeys.WriteTo(kf)
	require.NoError(t, err)

	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis --gen-keys --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)
}

func TestEvmGenesis_WritesGenesisToSpecifiedOutputLocation(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	err := os.MkdirAll(filepath.Join(homeDir, evmDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, evmDir, "n1"), 0700)
	require.NoError(t, err)

	nodeGenesisFile := filepath.Join(homeDir, evmDir, "n1", evmGenesisFileName)

	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis --gen-keys -o " + nodeGenesisFile + " --home " + homeDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, filepath.Join(homeDir, evmDir, defaultKeysFileName))
	require.FileExists(t, nodeGenesisFile)
}

func TestEvmGenesis_WithSystemIdentifier(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	err := os.MkdirAll(filepath.Join(homeDir, evmDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, evmDir, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, evmDir, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, evmDir, "n1", evmGenesisFileName)

	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis -g -k " + kf + " -o " + nodeGenesisFile + " -s 01010101"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	require.True(t, bytes.Equal([]byte{1, 1, 1, 1}, pn.BlockCertificationRequest.SystemIdentifier))
}

func TestEvmGenesis_WithParameters(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	err := os.MkdirAll(filepath.Join(homeDir, evmDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, evmDir, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, evmDir, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, evmDir, "n1", evmGenesisFileName)

	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis -g -k " + kf + " -o " + nodeGenesisFile + " --gas-limit=100000 --gas-price=1111111"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	require.FileExists(t, kf)
	require.FileExists(t, nodeGenesisFile)

	pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{})
	require.NoError(t, err)
	params := &genesis.EvmPartitionParams{}
	require.NoError(t, cbor.Unmarshal(pn.Params, params))
	require.EqualValues(t, 100000, params.BlockGasLimit)
	require.EqualValues(t, 1111111, params.GasUnitPrice)
}

func TestEvmGenesis_WithParameters_ErrorGasPriceTooBig(t *testing.T) {
	homeDir := setupTestHomeDir(t, evmDir)
	err := os.MkdirAll(filepath.Join(homeDir, evmDir), 0700)
	require.NoError(t, err)

	err = os.MkdirAll(filepath.Join(homeDir, evmDir, "n1"), 0700)
	require.NoError(t, err)

	kf := filepath.Join(homeDir, evmDir, "n1", defaultKeysFileName)
	nodeGenesisFile := filepath.Join(homeDir, evmDir, "n1", evmGenesisFileName)

	cmd := New(logger.LoggerBuilder(t))
	args := "evm-genesis -g -k " + kf + " -o " + nodeGenesisFile + " --gas-limit=100000 --gas-price=9223372036854775808"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, "gas unit price too big")
}

func Test_getPartitionParams_PriceTooBig(t *testing.T) {
	c := &evmGenesisConfig{
		BlockGasLimit: evm.DefaultBlockGasLimit,
		GasUnitPrice:  math.MaxInt64 + 1,
	}
	params, err := c.getPartitionParams()
	require.ErrorContains(t, err, "gas unit price too big")
	require.Nil(t, params)
}

func setupTestHomeDir(t *testing.T, dir string) string {
	outputDir := filepath.Join(t.TempDir(), dir)
	err := os.MkdirAll(outputDir, 0700) // -rwx------
	require.NoError(t, err)
	return outputDir
}
