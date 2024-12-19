package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/util"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

type consensusParams struct {
	totalNodes uint8
}

// Happy path
func TestGenerateDistributedGenesisFiles(t *testing.T) {
	homeDir := t.TempDir()
	logF := testobserve.NewFactory(t)
	consensus := consensusParams{totalNodes: 4}
	genesisFiles := createRootGenesisFiles(t, homeDir, consensus)
	genesisArg := fmt.Sprintf("%s,%s,%s,%s", genesisFiles[0], genesisFiles[1], genesisFiles[2], genesisFiles[3])
	// merge to distributed root genesis
	outputDir := filepath.Join(homeDir, "result")
	cmd := New(logF)
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=" + genesisArg
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))
	rootGenesis, err := util.ReadJsonFile(filepath.Join(outputDir, rootGenesisFileName), &genesis.RootGenesis{Version: 1})
	require.NoError(t, err)
	require.Len(t, rootGenesis.Root.RootValidators, 4)
	require.NoError(t, rootGenesis.Verify())
	partitionGenesis, err := util.ReadJsonFile(filepath.Join(outputDir, "partition-genesis-1.json"), &genesis.PartitionGenesis{})
	require.NoError(t, err)
	require.Len(t, partitionGenesis.RootValidators, 4)
	trustBase, err := partitionGenesis.GenerateRootTrustBase()
	require.NoError(t, err)
	require.NoError(t, partitionGenesis.IsValid(trustBase, crypto.SHA256))
	// iterate over key files and make sure that they are present
	// FindPubKeyById returns matching PublicKeyInfo matching node id or nil if not found
	findPubKeyFn := func(id string, keys []*genesis.PublicKeyInfo) *genesis.PublicKeyInfo {
		if keys == nil {
			return nil
		}
		// linear search for id
		for _, info := range keys {
			if info.NodeID == id {
				return info
			}
		}
		return nil
	}
	for i := 0; i < int(consensus.totalNodes); i++ {
		rootNodeDir := filepath.Join(homeDir, defaultRootChainDir+strconv.Itoa(i))
		keys, err := LoadKeys(filepath.Join(rootNodeDir, defaultKeysFileName), false, false)
		require.NoError(t, err)
		id, err := peer.IDFromPublicKey(keys.AuthPrivKey.GetPublic())
		require.NoError(t, err)
		require.NotNil(t, rootGenesis.Root.FindPubKeyById(id.String()))
		// make sure the root node is also present in partition genesis
		require.NotNil(t, findPubKeyFn(id.String(), partitionGenesis.RootValidators))
	}
}

func TestDistributedGenesisFiles_DifferentRootConsensus(t *testing.T) {
	homeDir := t.TempDir()
	homeDir2 := t.TempDir()
	genesisFiles := createRootGenesisFiles(t, homeDir, consensusParams{totalNodes: 4})
	differentGenesisFiles := createRootGenesisFiles(t, homeDir2, consensusParams{totalNodes: 2})
	outputDir := filepath.Join(homeDir, "result")
	cmd := New(testobserve.NewFactory(t))
	genesisArg := fmt.Sprintf("%s,%s,%s,%s", differentGenesisFiles[0], genesisFiles[1], genesisFiles[2], genesisFiles[3])
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=" + genesisArg
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.ErrorContains(t, cmd.Execute(context.Background()), "root genesis merge failed: not compatible root genesis files, consensus is different")
}

func TestDistributedGenesisFiles_DuplicateRootNode(t *testing.T) {
	homeDir := t.TempDir()
	genesisFiles := createRootGenesisFiles(t, homeDir, consensusParams{totalNodes: 4})
	// merge distributed root genesis file
	outputDir := filepath.Join(homeDir, "result")
	// genesis argument, repeat root node 2 genesis
	genesisArg := fmt.Sprintf("%s,%s,%s,%s", genesisFiles[0], genesisFiles[1], genesisFiles[2], genesisFiles[2])
	cmd := New(testobserve.NewFactory(t))
	args := "root-genesis combine --home " + homeDir +
		" -o " + outputDir +
		" --root-genesis=" + genesisArg
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))
	rootGenesis, err := util.ReadJsonFile(filepath.Join(outputDir, rootGenesisFileName), &genesis.RootGenesis{Version: 1})
	require.NoError(t, err)
	// duplicate is ignored
	require.Len(t, rootGenesis.Root.RootValidators, 3)
	require.ErrorContains(t, rootGenesis.Verify(), "root genesis record error: registered root nodes do not match consensus total root nodes")
}

func Test_RootGenesis_New_Sign(t *testing.T) {
	// create partition genesis to be used with the tests
	homeDir := t.TempDir()
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName)
	cmd := New(testobserve.NewFactory(t))
	// create money node genesis
	pdrFilename, err := createPDRFile(homeDir, defaultMoneyPDR)
	require.NoError(t, err)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g --partition-description " + pdrFilename
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	t.Run("GenerateGenesisFilesAndSign", func(t *testing.T) {
		homeDir := t.TempDir()
		outputDirNode1 := filepath.Join(homeDir, defaultRootChainDir+"1")
		// create root node 1 genesis with root node
		logF := testobserve.NewFactory(t)
		cmd = New(logF)
		args = "root-genesis new --home " + homeDir +
			" -o " + outputDirNode1 +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" --total-nodes=2" +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		// create key for node 2 and sign genesis
		outputDirNode2 := filepath.Join(homeDir, defaultRootChainDir+"2")
		cmd = New(logF)
		args = "root-genesis sign --home " + homeDir + " -o " + outputDirNode2 + " -g" + " --root-genesis=" + filepath.Join(outputDirNode1, rootGenesisFileName)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		// read genesis file
		rootGenesis, err := util.ReadJsonFile(filepath.Join(outputDirNode2, rootGenesisFileName), &genesis.RootGenesis{Version: 1})
		require.NoError(t, err)
		require.NoError(t, rootGenesis.Verify())
	})

	t.Run("GenerateGenesisFilesAndSign_ErrTooMany", func(t *testing.T) {
		homeDir := t.TempDir()
		logF := testobserve.NewFactory(t)
		outputDirNode1 := filepath.Join(homeDir, defaultRootChainDir+"1")
		// create root node 1 genesis with root node
		cmd := New(logF)
		args = "root-genesis new --home " + homeDir +
			" -o " + outputDirNode1 +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" --total-nodes=2" +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		// create key for node 2 and sign genesis
		outputDirNode2 := filepath.Join(homeDir, defaultRootChainDir+"2")
		cmd = New(logF)
		args = "root-genesis sign --home " + homeDir + " -o " + outputDirNode2 + " -g" + " --root-genesis=" + filepath.Join(outputDirNode1, rootGenesisFileName)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		// create key for node 3 and sign genesis
		outputDirNode3 := filepath.Join(homeDir, defaultRootChainDir+"3")
		cmd = New(logF)
		args = "root-genesis sign --home " + homeDir + " -o " + outputDirNode3 + " -g" + " --root-genesis=" + filepath.Join(outputDirNode2, rootGenesisFileName)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.ErrorContains(t, cmd.Execute(context.Background()), "root genesis add signature failed: genesis is already signed by maximum number of root nodes")
	})
}

func createRootGenesisFiles(t *testing.T, homeDir string, params consensusParams) []string {
	t.Helper()
	logF := testobserve.NewFactory(t)
	cmd := New(logF)
	// create money node genesis
	pdrFilename, err := createPDRFile(homeDir, defaultMoneyPDR)
	require.NoError(t, err)
	nodeGenesisFileLocation := filepath.Join(homeDir, moneyPartitionDir, moneyGenesisFileName)
	nodeKeysFileLocation := filepath.Join(homeDir, moneyPartitionDir, defaultKeysFileName)
	args := "money-genesis --home " + homeDir + " -o " + nodeGenesisFileLocation + " -g -k " + nodeKeysFileLocation + " --partition-description " + pdrFilename
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	require.NoError(t, cmd.Execute(context.Background()))

	totalNodesStr := strconv.Itoa(int(params.totalNodes))
	genesisFiles := make([]string, params.totalNodes)
	// create number of root genesis files
	for i := 0; i < int(params.totalNodes); i++ {
		rootNodeDir := filepath.Join(homeDir, defaultRootChainDir+strconv.Itoa(i))
		// create root node 1 genesis with root node
		cmd = New(logF)
		args = "root-genesis new --home " + homeDir +
			" -o " + rootNodeDir +
			" --partition-node-genesis-file=" + nodeGenesisFileLocation +
			" --total-nodes=" + totalNodesStr +
			" -g"
		cmd.baseCmd.SetArgs(strings.Split(args, " "))
		require.NoError(t, cmd.Execute(context.Background()))
		genesisFiles[i] = filepath.Join(rootNodeDir, rootGenesisFileName)
	}
	return genesisFiles
}
