package cmd

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
)

func TestGenerateGenesisFiles_OK(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create partition genesis file (e.g. money)
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
	cmd := New(logF)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	rootDir := filepath.Join(homeDir, defaultRootChainDir)
	// create root node 1 genesis with root node
	cmd = New(logF)
	args = "root-genesis new --home " + homeDir +
		" -o " + rootDir +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	// read resulting file
	rootGenesis, err := util.ReadJsonFile(filepath.Join(rootDir, rootGenesisFileName), &genesis.RootGenesis{})
	require.NoError(t, err)
	require.Len(t, rootGenesis.Root.RootValidators, 1)
	require.NoError(t, rootGenesis.Verify())
	partitionGenesis, err := util.ReadJsonFile(filepath.Join(rootDir, "partition-genesis-0.json"), &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.Len(t, partitionGenesis.RootValidators, 1)
	trustBase, err := genesis.NewValidatorTrustBase(partitionGenesis.RootValidators)
	require.NoError(t, partitionGenesis.IsValid(trustBase, gocrypto.SHA256))
	// verify root consensus parameters, using defaults
	require.EqualValues(t, 1, rootGenesis.Root.Consensus.TotalRootValidators)
	require.EqualValues(t, 1, rootGenesis.Root.Consensus.QuorumThreshold)
	require.EqualValues(t, 900, rootGenesis.Root.Consensus.BlockRateMs)
	require.EqualValues(t, 10000, rootGenesis.Root.Consensus.ConsensusTimeoutMs)
	require.Len(t, rootGenesis.Partitions, 1)
	// verify, content
	require.Len(t, rootGenesis.Partitions[0].Nodes, 1)
	require.EqualValues(t, rootGenesis.Partitions[0].SystemDescriptionRecord.SystemIdentifier, money.DefaultSystemIdentifier)
	//
}

func TestRootGenesis_KeyFileNotFound(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create partition genesis file (e.g. money)
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
	cmd := New(logF)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	rootDir := filepath.Join(homeDir, defaultRootChainDir)
	// create root node 1 genesis with root node
	cmd = New(logF)
	args = "root-genesis new --home " + homeDir +
		" -o " + rootDir +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	s := filepath.Join(homeDir, defaultRootChainDir, defaultKeysFileName)
	require.ErrorContains(t, cmd.addAndExecuteCommand(context.Background()), fmt.Sprintf("failed to read root chain keys from file '%s'", s))
}

func TestRootGenesis_ForceKeyGeneration(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create partition genesis file (e.g. money)
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
	cmd := New(logF)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	rootDir := filepath.Join(homeDir, defaultRootChainDir)
	// create root node 1 genesis with root node
	cmd = New(logF)
	args = "root-genesis new --gen-keys --home " + homeDir +
		" -o " + rootDir +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(homeDir, defaultRootChainDir, defaultKeysFileName))
}

func TestGenerateGenesisFiles_InvalidPartitionSignature(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create partition genesis file (e.g. money)
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
	cmd := New(logF)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	// invalidate the generated file signature
	moneyGenesis, err := util.ReadJsonFile(filepath.Join(nodeGenesisFileLocation), &genesis.PartitionNode{})
	require.NoError(t, err)
	copy(moneyGenesis.BlockCertificationRequest.Signature[:], []byte{'F', 'O', 'O', 'B', 'A', 'R'})
	require.NoError(t, util.WriteJsonFile(filepath.Join(nodeGenesisFileLocation), moneyGenesis))
	// create root node genesis with root node
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd = New(logF)
	args = "root-genesis new --home " + homeDir +
		" -o " + genesisFileDir +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.addAndExecuteCommand(context.Background()), "signature verification failed")
}

func TestGenerateGenesisFiles_ErrOnlyGenerateKeyFile(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create root node genesis with root node 1
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(logF)
	args := "root-genesis new --home " + homeDir +
		" -o " + genesisFileDir + " -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.addAndExecuteCommand(context.Background()), `required flag(s) "partition-node-genesis-file"`)
}

func TestGenerateGenesisFiles_ErrNoNodeGenesisFilesNorGenerateKeys(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	// create root node genesis with root node 1
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(logF)
	args := "root-genesis new --home " + homeDir +
		" -o " + genesisFileDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.addAndExecuteCommand(context.Background()), `required flag(s) "partition-node-genesis-file"`)
}
