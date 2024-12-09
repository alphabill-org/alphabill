package cmd

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/txsystem/evm"
)

func Test_EvmGenesis(t *testing.T) {
	// create partition description file to be shared in all the tests
	pdr := types.PartitionDescriptionRecord{
		Version:           1,
		NetworkID: 5,
		PartitionID:       evmsdk.DefaultPartitionID,
		TypeIDLen:         8,
		UnitIDLen:         256,
		T2Timeout:         2500 * time.Millisecond,
	}
	pdrFilename, err := createPDRFile(t.TempDir(), &pdr)
	require.NoError(t, err)

	t.Run("KeyFileNotFound", func(t *testing.T) {
		homeDir := t.TempDir()
		args := "evm-genesis --home " + homeDir + " --partition-description " + pdrFilename
		cmd := New(testobserve.NewFactory(t))
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("load keys %s failed", filepath.Join(homeDir, evmDir, defaultKeysFileName)))
	})

	t.Run("ForceKeyGeneration", func(t *testing.T) {
		homeDir := t.TempDir()
		args := "evm-genesis --gen-keys --home " + homeDir + " --partition-description " + pdrFilename
		cmd := New(testobserve.NewFactory(t))
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(homeDir, evmDir, defaultKeysFileName))
		require.FileExists(t, filepath.Join(homeDir, evmDir, evmGenesisFileName))
	})

	t.Run("DefaultNodeGenesisExists", func(t *testing.T) {
		homeDir := t.TempDir()
		nodeGenesisFile := filepath.Join(homeDir, evmDir, evmGenesisFileName)
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, evmDir), 0700))
		require.NoError(t, util.WriteJsonFile(nodeGenesisFile, &genesis.PartitionNode{Version: 1, NodeID: "1"}))

		cmd := New(testobserve.NewFactory(t))
		args := "evm-genesis --gen-keys --home " + homeDir + " --partition-description " + pdrFilename
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, fmt.Sprintf("node genesis file %q already exists", nodeGenesisFile))
		require.NoFileExists(t, filepath.Join(homeDir, evmDir, defaultKeysFileName))
	})

	t.Run("LoadExistingKeys", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, evmDir), 0700))
		keysFilename := filepath.Join(homeDir, evmDir, defaultKeysFileName)
		nodeGenesisFile := filepath.Join(homeDir, evmDir, evmGenesisFileName)

		nodeKeys, err := GenerateKeys()
		require.NoError(t, err)
		require.NoError(t, nodeKeys.WriteTo(keysFilename))

		cmd := New(testobserve.NewFactory(t))
		args := "evm-genesis --gen-keys --home " + homeDir + " --partition-description " + pdrFilename
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err = cmd.Execute(context.Background())
		require.NoError(t, err)

		require.FileExists(t, keysFilename)
		require.FileExists(t, nodeGenesisFile)
	})

	t.Run("WritesGenesisToSpecifiedOutputLocation", func(t *testing.T) {
		homeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, evmDir, "n1"), 0700))

		nodeGenesisFile := filepath.Join(homeDir, evmDir, "n1", evmGenesisFileName)
		nodeGenesisStateFile := filepath.Join(homeDir, evmDir, "n1", evmGenesisStateFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "evm-genesis --gen-keys -o " + nodeGenesisFile + " --output-state " + nodeGenesisStateFile + " --home " + homeDir + " --partition-description " + pdrFilename
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, filepath.Join(homeDir, evmDir, defaultKeysFileName))
		require.FileExists(t, nodeGenesisFile)
		require.FileExists(t, nodeGenesisStateFile)
	})

	t.Run("WithParameters", func(t *testing.T) {
		homeDir := t.TempDir()
		kf := filepath.Join(homeDir, evmDir, defaultKeysFileName)
		nodeGenesisFile := filepath.Join(homeDir, evmDir, evmGenesisFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "evm-genesis -g -k " + kf + " -o " + nodeGenesisFile + " --gas-limit=100000 --gas-price=1111111" + " --home " + homeDir + " --partition-description " + pdrFilename
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		require.FileExists(t, kf)
		require.FileExists(t, nodeGenesisFile)

		pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{Version: 1})
		require.NoError(t, err)
		params := &genesis.EvmPartitionParams{}
		require.NoError(t, types.Cbor.Unmarshal(pn.Params, params))
		require.EqualValues(t, 100000, params.BlockGasLimit)
		require.EqualValues(t, 1111111, params.GasUnitPrice)
	})

	t.Run("WithParameters_ErrorGasPriceTooBig", func(t *testing.T) {
		homeDir := t.TempDir()
		kf := filepath.Join(homeDir, evmDir, defaultKeysFileName)
		nodeGenesisFile := filepath.Join(homeDir, evmDir, evmGenesisFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "evm-genesis -g -k " + kf + " -o " + nodeGenesisFile + " --gas-limit=100000 --gas-price=9223372036854775808" + " --home " + homeDir + " --partition-description " + pdrFilename
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		err := cmd.Execute(context.Background())
		require.ErrorContains(t, err, "gas unit price too big")
	})

	t.Run("partition description is loaded", func(t *testing.T) {
		homeDir := t.TempDir()
		nodeGenesisFile := filepath.Join(homeDir, evmDir, evmGenesisFileName)

		cmd := New(testobserve.NewFactory(t))
		args := "evm-genesis -g -o " + nodeGenesisFile + " --home " + homeDir + " --partition-description " + pdrFilename
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))

		pn, err := util.ReadJsonFile(nodeGenesisFile, &genesis.PartitionNode{Version: 1})
		require.NoError(t, err)
		require.EqualValues(t, pdr, pn.PartitionDescriptionRecord)
		require.EqualValues(t, pdr.PartitionID, pn.BlockCertificationRequest.PartitionID)
	})
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
