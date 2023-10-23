package cmd

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

type consensusParams struct {
	totalNodes uint8
	threshold  uint8
}

// Happy path
func TestGenerateDistributedGenesisFiles(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	consensus := consensusParams{totalNodes: 4, threshold: 3}
	genesisFiles := createRootGenesisFiles(t, homeDir, consensus)
	genesisArg := fmt.Sprintf("%s,%s,%s,%s", genesisFiles[0], genesisFiles[1], genesisFiles[2], genesisFiles[3])
	// merge to distributed root genesis
	outputDir := filepath.Join(homeDir, "result")
	cmd := New(logF)
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=" + genesisArg
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	rootGenesis, err := util.ReadJsonFile(filepath.Join(outputDir, rootGenesisFileName), &genesis.RootGenesis{})
	require.NoError(t, err)
	require.Len(t, rootGenesis.Root.RootValidators, 4)
	require.NoError(t, rootGenesis.Verify())
	partitionGenesis, err := util.ReadJsonFile(filepath.Join(outputDir, "partition-genesis-0.json"), &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.Len(t, partitionGenesis.RootValidators, 4)
	trustBase, err := genesis.NewValidatorTrustBase(partitionGenesis.RootValidators)
	require.NoError(t, partitionGenesis.IsValid(trustBase, gocrypto.SHA256))
	// iterate over key files and make sure that they are present
	// FindPubKeyById returns matching PublicKeyInfo matching node id or nil if not found
	findPubKeyFn := func(id string, keys []*genesis.PublicKeyInfo) *genesis.PublicKeyInfo {
		if keys == nil {
			return nil
		}
		// linear search for id
		for _, info := range keys {
			if info.NodeIdentifier == id {
				return info
			}
		}
		return nil
	}
	for i := uint8(0); i < consensus.totalNodes; i++ {
		rootNodeDir := filepath.Join(homeDir, defaultRootChainDir+strconv.Itoa(int(i)))
		keys, err := LoadKeys(filepath.Join(rootNodeDir, defaultKeysFileName), false, false)
		require.NoError(t, err)
		id, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
		require.NoError(t, err)
		require.NotNil(t, rootGenesis.Root.FindPubKeyById(id.String()))
		// make sure the root node is also present in partition genesis
		require.NotNil(t, findPubKeyFn(id.String(), partitionGenesis.RootValidators))
	}
}

func TestDistributedGenesisFiles_DifferentRootConsensus(t *testing.T) {
	homeDir := t.TempDir()
	homeDir2 := t.TempDir()
	logF := logger.LoggerBuilder(t)
	genesisFiles := createRootGenesisFiles(t, homeDir, consensusParams{totalNodes: 4, threshold: 4})
	differentGenesisFiles := createRootGenesisFiles(t, homeDir2, consensusParams{totalNodes: 2, threshold: 2})
	outputDir := filepath.Join(homeDir, "result")
	cmd := New(logF)
	genesisArg := fmt.Sprintf("%s,%s,%s,%s", differentGenesisFiles[0], genesisFiles[1], genesisFiles[2], genesisFiles[2])
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=" + genesisArg
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.addAndExecuteCommand(context.Background()), "root genesis merge failed, not compatible root genesis files, consensus is different")
}

func TestDistributedGenesisFiles_DuplicateRootNode(t *testing.T) {
	homeDir := t.TempDir()
	logF := logger.LoggerBuilder(t)
	genesisFiles := createRootGenesisFiles(t, homeDir, consensusParams{totalNodes: 4, threshold: 3})
	// merge distributed root genesis file
	outputDir := filepath.Join(homeDir, "result")
	// genesis argument, repeat root node 2 genesis
	genesisArg := fmt.Sprintf("%s,%s,%s,%s", genesisFiles[0], genesisFiles[1], genesisFiles[2], genesisFiles[2])
	cmd := New(logF)
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=" + genesisArg
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	rootGenesis, err := util.ReadJsonFile(filepath.Join(outputDir, rootGenesisFileName), &genesis.RootGenesis{})
	require.NoError(t, err)
	// duplicate is ignored
	require.Len(t, rootGenesis.Root.RootValidators, 3)
	require.ErrorContains(t, rootGenesis.Verify(), "root genesis record error: registered root nodes do not match consensus total root nodes")
}

func TestGenerateGenesisFilesAndSign(t *testing.T) {
	homeDir := t.TempDir()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	logF := logger.LoggerBuilder(t)
	cmd := New(logF)
	// create money node genesis
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	outputDirNode1 := filepath.Join(homeDir, defaultRootChainDir+"1")
	// create root node 1 genesis with root node
	cmd = New(logF)
	args = "root-genesis new --home " + homeDir +
		" -o " + outputDirNode1 +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" --total-nodes=2" +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	// create key for node 2 and sign genesis
	outputDirNode2 := filepath.Join(homeDir, defaultRootChainDir+"2")
	cmd = New(logF)
	args = "root-genesis sign --home " + homeDir + " -o " + outputDirNode2 + " -g" + " --root-genesis=" + filepath.Join(outputDirNode1, rootGenesisFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	// read genesis file
	rootGenesis, err := util.ReadJsonFile(filepath.Join(outputDirNode2, rootGenesisFileName), &genesis.RootGenesis{})
	require.NoError(t, err)
	require.NoError(t, rootGenesis.Verify())
}

func TestGenerateGenesisFilesAndSign_ErrTooMany(t *testing.T) {
	homeDir := t.TempDir()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	logF := logger.LoggerBuilder(t)
	cmd := New(logF)
	// create money node genesis
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	outputDirNode1 := filepath.Join(homeDir, defaultRootChainDir+"1")
	// create root node 1 genesis with root node
	cmd = New(logF)
	args = "root-genesis new --home " + homeDir +
		" -o " + outputDirNode1 +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" --total-nodes=2" +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	// create key for node 2 and sign genesis
	outputDirNode2 := filepath.Join(homeDir, defaultRootChainDir+"2")
	cmd = New(logF)
	args = "root-genesis sign --home " + homeDir + " -o " + outputDirNode2 + " -g" + " --root-genesis=" + filepath.Join(outputDirNode1, rootGenesisFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	// create key for node 3 and sign genesis
	outputDirNode3 := filepath.Join(homeDir, defaultRootChainDir+"3")
	cmd = New(logF)
	args = "root-genesis sign --home " + homeDir + " -o " + outputDirNode3 + " -g" + " --root-genesis=" + filepath.Join(outputDirNode2, rootGenesisFileName)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.addAndExecuteCommand(context.Background()), "root genesis add signature error, genesis is already signed by maximum number of root nodes")
}

func createRootGenesisFiles(t *testing.T, homeDir string, params consensusParams) []string {
	t.Helper()
	logF := logger.LoggerBuilder(t)
	cmd := New(logF)
	// create money node genesis
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyGenesisDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyGenesisDir, defaultKeysFileName)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	outputDirNode1 := filepath.Join(homeDir, defaultRootChainDir+"1")
	// create root node 1 genesis with root node
	cmd = New(logF)
	args = "root-genesis new --home " + homeDir +
		" -o " + outputDirNode1 +
		" --partition-node-genesis-file=" + nodeGenesisFileLocation +
		" --total-nodes=2" +
		" -g"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
	totalNodesStr := strconv.Itoa(int(params.totalNodes))
	threshold := strconv.Itoa(int(params.threshold))
	genesisFiles := make([]string, params.totalNodes)
	// create number of root genesis files
	for i := uint8(0); i < params.totalNodes; i++ {
		rootNodeDir := filepath.Join(homeDir, defaultRootChainDir+strconv.Itoa(int(i)))
		// create root node 1 genesis with root node
		cmd = New(logF)
		args = "root-genesis new --home " + homeDir +
			" -o " + rootNodeDir +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" --total-nodes=" + totalNodesStr +
			" --quorum-threshold=" + threshold +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.addAndExecuteCommand(context.Background()))
		genesisFiles[i] = filepath.Join(rootNodeDir, rootGenesisFileName)
	}
	return genesisFiles
}
