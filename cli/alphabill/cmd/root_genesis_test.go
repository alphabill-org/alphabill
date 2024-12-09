package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

func Test_RootGenesis_New(t *testing.T) {
	// create partition genesis to be used with the tests
	homeDir := t.TempDir()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName)
	pdrFilename, err := createPDRFile(homeDir, defaultMoneyPDR)
	require.NoError(t, err)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g --partition-description " + pdrFilename
	cmd := New(testobserve.NewFactory(t))
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	t.Run("happy path", func(t *testing.T) {
		homeDir := t.TempDir()
		rootDir := filepath.Join(homeDir, defaultRootChainDir)
		// create root node 1 genesis with root node
		cmd := New(testobserve.NewFactory(t))
		args = "root-genesis new --home " + homeDir +
			" -o " + rootDir +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		// read resulting file
		rootGenesis, err := util.ReadJsonFile(filepath.Join(rootDir, rootGenesisFileName), &genesis.RootGenesis{Version: 1})
		require.NoError(t, err)
		require.Len(t, rootGenesis.Root.RootValidators, 1)
		require.NoError(t, rootGenesis.Verify())
		partitionGenesis, err := util.ReadJsonFile(filepath.Join(rootDir, "partition-genesis-1.json"), &genesis.PartitionGenesis{})
		require.NoError(t, err)
		require.Len(t, partitionGenesis.RootValidators, 1)
		trustBase, err := partitionGenesis.GenerateRootTrustBase()
		require.NoError(t, err)
		require.NoError(t, partitionGenesis.IsValid(trustBase, crypto.SHA256))
		// verify root consensus parameters, using defaults
		require.EqualValues(t, 1, rootGenesis.Root.Consensus.TotalRootValidators)
		require.EqualValues(t, 900, rootGenesis.Root.Consensus.BlockRateMs)
		require.EqualValues(t, 10000, rootGenesis.Root.Consensus.ConsensusTimeoutMs)
		require.Len(t, rootGenesis.Partitions, 1)
		// verify, content
		require.Len(t, rootGenesis.Partitions[0].Nodes, 1)
		require.EqualValues(t, rootGenesis.Partitions[0].PartitionDescription.PartitionID, money.DefaultPartitionID)
	})

	t.Run("KeyFileNotFound", func(t *testing.T) {
		homeDir := t.TempDir()
		// create root node 1 genesis with root node
		cmd := New(testobserve.NewFactory(t))
		args = "root-genesis new --home " + homeDir +
			" -o " + homeDir +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		s := filepath.Join(homeDir, defaultKeysFileName)
		require.ErrorContains(t, cmd.Execute(context.Background()), fmt.Sprintf("failed to read root chain keys from file '%s'", s))
	})

	t.Run("ForceKeyGeneration", func(t *testing.T) {
		homeDir := t.TempDir()
		rootDir := filepath.Join(homeDir, defaultRootChainDir)
		// create root node 1 genesis with root node
		cmd := New(testobserve.NewFactory(t))
		args = "root-genesis new --gen-keys --home " + homeDir +
			" -o " + rootDir +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		require.FileExists(t, filepath.Join(homeDir, defaultRootChainDir, defaultKeysFileName))
	})

	t.Run("InvalidPartitionSignature", func(t *testing.T) {
		homeDir := t.TempDir()
		// create partition node genesis file with invalid signature
		moneyGenesis, err := util.ReadJsonFile(nodeGenesisFileLocation, &genesis.PartitionNode{Version: 1})
		require.NoError(t, err)
		copy(moneyGenesis.BlockCertificationRequest.Signature[:], []byte{'F', 'O', 'O', 'B', 'A', 'R'})
		invalidPGFile := filepath.Join(homeDir, "invalid-pg.json")
		require.NoError(t, util.WriteJsonFile(invalidPGFile, moneyGenesis))
		// create root node genesis with root node
		cmd := New(testobserve.NewFactory(t))
		args = "root-genesis new --home " + homeDir +
			" -o " + homeDir +
			" --partition-node-genesis-file=" + invalidPGFile +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.ErrorContains(t, cmd.Execute(context.Background()), "signature verification: verification failed")
	})

	t.Run("ErrBlockRateInvalid", func(t *testing.T) {
		homeDir := t.TempDir()
		rootDir := filepath.Join(homeDir, defaultRootChainDir)
		// create root node 1 genesis with root node
		cmd := New(testobserve.NewFactory(t))
		args = "root-genesis new --home " + homeDir +
			" -o " + rootDir +
			" --block-rate=10" +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.ErrorContains(t, cmd.Execute(context.Background()), "consensus parameters validation failed: invalid block rate, must be at least 100")
	})

	t.Run("ErrTimeoutInvalid", func(t *testing.T) {
		homeDir := t.TempDir()
		rootDir := filepath.Join(homeDir, defaultRootChainDir)
		// create root node 1 genesis with root node
		cmd := New(testobserve.NewFactory(t))
		args = "root-genesis new --home " + homeDir +
			" -o " + rootDir +
			" --block-rate=500" +
			" --consensus-timeout=200" +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.ErrorContains(t, cmd.Execute(context.Background()), "consensus parameters validation failed: invalid consensus timeout, must be at least 2000")
	})

	t.Run("ErrBlockRateBiggerThanTimeout", func(t *testing.T) {
		homeDir := t.TempDir()
		rootDir := filepath.Join(homeDir, defaultRootChainDir)
		// create root node 1 genesis with root node
		cmd := New(testobserve.NewFactory(t))
		args = "root-genesis new --home " + homeDir +
			" -o " + rootDir +
			" --block-rate=2500" +
			" --consensus-timeout=2000" +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.ErrorContains(t, cmd.Execute(context.Background()), "consensus parameters validation failed: invalid timeout for block rate, must be at least 4500")
	})
}

func TestGenerateGenesisFiles_ErrOnlyGenerateKeyFile(t *testing.T) {
	homeDir := t.TempDir()
	// create root node genesis with root node 1
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(testobserve.NewFactory(t))
	args := "root-genesis new --home " + homeDir +
		" -o " + genesisFileDir + " -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.Execute(context.Background()), `required flag(s) "partition-node-genesis-file"`)
}

func TestGenerateGenesisFiles_ErrNoNodeGenesisFilesNorGenerateKeys(t *testing.T) {
	homeDir := t.TempDir()
	// create root node genesis with root node 1
	genesisFileDir := filepath.Join(homeDir, defaultRootChainDir)
	cmd := New(testobserve.NewFactory(t))
	args := "root-genesis new --home " + homeDir +
		" -o " + genesisFileDir
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.Execute(context.Background()), `required flag(s) "partition-node-genesis-file"`)
}
