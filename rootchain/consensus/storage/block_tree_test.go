package storage

import (
	"crypto"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	testpartition "github.com/alphabill-org/alphabill/rootchain/partitions/testutils"
)

func mockExecutedBlock(round, qcRound, qcParentRound uint64) *ExecutedBlock {
	var randomHash = test.RandomBytes(32)
	return &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Round: round,
			Qc: &drctypes.QuorumCert{
				VoteInfo: &drctypes.RoundInfo{
					RoundNumber:       qcRound,
					ParentRoundNumber: qcParentRound,
					Epoch:             0,
					CurrentRootHash:   randomHash,
				},
				LedgerCommitInfo: &types.UnicitySeal{
					Version: 1,
					Hash:    randomHash,
				},
			},
		},
		ShardState: ShardStates{States: map[types.PartitionShardID]*ShardInfo{}},
		HashAlgo:   crypto.SHA256,
	}
}

func hexToBytes(hexStr string) []byte {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(fmt.Sprintf("error decoding hex string: %s", err))
	}
	return b
}

/*
createTestBlockTree creates the following tree

	 ╭--> B6--> B7--> B8
	B5--> B9--> B10
	       ╰--> B11--> B12
*/
func createTestBlockTree(t *testing.T) *BlockTree {
	treeNodes := make(map[uint64]*node)
	db, err := memorydb.New()
	require.NoError(t, err)
	rootNode := &node{data: &ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B5", Round: 5}}}
	b6 := newNode(&ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B6", Round: 6, Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 5}}}})
	b7 := newNode(&ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B7", Round: 7, Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 6}}}})
	b8 := newNode(&ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B8", Round: 8, Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}}}})
	b9 := newNode(&ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B9", Round: 9, Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 5}}}})
	b10 := newNode(&ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B10", Round: 10, Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}}})
	b11 := newNode(&ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B11", Round: 11, Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}}})
	b12 := newNode(&ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B12", Round: 12, Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 11}}}})
	// init tree structure
	// root has two children B6 and B9
	rootNode.addChild(b6)
	rootNode.addChild(b9)
	// B6--> B7
	b6.addChild(b7)
	// B7--> B8
	b7.addChild(b8)
	// b9 has two children B10 and B11
	b9.addChild(b10)
	b9.addChild(b11)
	// B11--> B12
	b11.addChild(b12)
	treeNodes[5] = rootNode
	require.NoError(t, db.Write(blockKey(5), rootNode.data))
	treeNodes[6] = b6
	require.NoError(t, db.Write(blockKey(6), b6.data))
	treeNodes[7] = b7
	require.NoError(t, db.Write(blockKey(7), b7.data))
	treeNodes[8] = b8
	require.NoError(t, db.Write(blockKey(8), b8.data))
	treeNodes[9] = b9
	require.NoError(t, db.Write(blockKey(9), b9.data))
	treeNodes[10] = b10
	require.NoError(t, db.Write(blockKey(10), b10.data))
	treeNodes[11] = b11
	require.NoError(t, db.Write(blockKey(11), b11.data))
	treeNodes[12] = b12
	require.NoError(t, db.Write(blockKey(12), b12.data))
	return &BlockTree{
		root:        rootNode,
		roundToNode: treeNodes,
		blocksDB:    db,
	}
}

func initFromGenesis(t *testing.T) *BlockTree {
	t.Helper()
	db, err := memorydb.New()
	require.NoError(t, err)

	require.NoError(t, storeGenesisInit(db, 5, crypto.SHA256))
	orchestration := testpartition.NewOrchestration(t, logger.New(t))
	btree, err := NewBlockTree(db, orchestration)
	require.NoError(t, err)
	return btree
}

func TestBlockTree_RemoveLeaf(t *testing.T) {
	tree := createTestBlockTree(t)
	// remove leaf that has children
	require.ErrorContains(t, tree.RemoveLeaf(9), "error round 9 is not leaf node")
	require.ErrorContains(t, tree.RemoveLeaf(5), "error root cannot be removed")
	require.ErrorContains(t, tree.RemoveLeaf(11), "error round 11 is not leaf node")
	require.NoError(t, tree.RemoveLeaf(8))
	b, err := tree.FindBlock(8)
	require.ErrorContains(t, err, "block for round 8 not found")
	require.Nil(t, b)
	b, err = tree.FindBlock(7)
	require.NoError(t, err)
	require.NotNil(t, b)
	require.Equal(t, "B7", b.BlockData.Author)
	// make sure node 7 child "8" was removed
	n, found := tree.roundToNode[7]
	require.True(t, found)
	require.Empty(t, n.child)
	// not and error if leaf does not exist, consider removed
	require.NoError(t, tree.RemoveLeaf(2))
}

func TestBlockTree_FindPathToRoot(t *testing.T) {
	tree := createTestBlockTree(t)
	// test tree root is set to B5
	blocks := tree.findPathToRoot(8)
	require.Len(t, blocks, 3)
	// B8-->B7-->B6-->root (not included)
	require.Equal(t, "B8", blocks[0].BlockData.Author)
	require.Equal(t, "B7", blocks[1].BlockData.Author)
	require.Equal(t, "B6", blocks[2].BlockData.Author)
	blocks = tree.findPathToRoot(10)
	// B10-->B9-->root (not included)
	require.Len(t, blocks, 2)
	require.Equal(t, "B10", blocks[0].BlockData.Author)
	require.Equal(t, "B9", blocks[1].BlockData.Author)
	blocks = tree.findPathToRoot(12)
	// B12-->B11-->B9-->root (not included)
	require.Len(t, blocks, 3)
	require.Equal(t, "B12", blocks[0].BlockData.Author)
	require.Equal(t, "B11", blocks[1].BlockData.Author)
	require.Equal(t, "B9", blocks[2].BlockData.Author)
	// node not found
	require.Nil(t, tree.findPathToRoot(2))
}

func TestBlockTree_GetAllUncommittedNodes(t *testing.T) {
	tree := createTestBlockTree(t)
	// tree has 8 nodes, only root is not committed
	require.Len(t, tree.roundToNode, 8)
	blocks := tree.GetAllUncommittedNodes()
	require.Len(t, blocks, 7)
	require.NoError(t, tree.RemoveLeaf(8))
	require.NoError(t, tree.RemoveLeaf(7))
	blocks = tree.GetAllUncommittedNodes()
	require.Len(t, blocks, 5)
}

func TestBlockTree_pruning(t *testing.T) {
	t.Run("prune from round 7", func(t *testing.T) {
		tree := createTestBlockTree(t)
		/*
		   ╭--> B6--> B7--> B8
		  B5--> B9--> B10
		         ╰--> B11--> B12
		*/
		// find blocks to prune if new committed root is B7
		rounds, err := tree.findBlocksToPrune(7)
		require.NoError(t, err)
		// only B7-->B8 shall remain, hence 6 will be removed
		require.Len(t, rounds, 6)
		require.ElementsMatch(t, rounds, []uint64{5, 6, 9, 10, 11, 12})
		require.NotContains(t, rounds, uint64(7))
		require.NotContains(t, rounds, uint64(8))
	})
	t.Run("prune from round 12", func(t *testing.T) {
		tree := createTestBlockTree(t)
		// find blocks to prune if new committed root is B12
		rounds, err := tree.findBlocksToPrune(12)
		require.NoError(t, err)
		// only B12 shall remain, hence 7 will be removed
		require.Len(t, rounds, 7)
		require.ElementsMatch(t, rounds, []uint64{5, 6, 7, 8, 9, 10, 11})
		require.NotContains(t, rounds, uint64(12))
	})
	t.Run("prune from round 9", func(t *testing.T) {
		tree := createTestBlockTree(t)
		// find blocks to prune if new committed root is B9
		rounds, err := tree.findBlocksToPrune(9)
		require.NoError(t, err)
		// B9-->10 and B9-->B11-->12 shall remain, hence 4 will be removed
		require.Len(t, rounds, 4)
		require.ElementsMatch(t, rounds, []uint64{5, 6, 7, 8})
		require.NotContains(t, rounds, uint64(9))
		require.NotContains(t, rounds, uint64(10))
		require.NotContains(t, rounds, uint64(11))
		require.NotContains(t, rounds, uint64(12))
	})
	t.Run("err - new root not found", func(t *testing.T) {
		tree := createTestBlockTree(t)
		// new root cannot be found
		rounds, err := tree.findBlocksToPrune(15)
		require.ErrorContains(t, err, "new root round 15 not found")
		require.Nil(t, rounds)
	})
	t.Run("no changes, old is also new root", func(t *testing.T) {
		tree := createTestBlockTree(t)
		// nothing gets pruned if new root is old root
		rounds, err := tree.findBlocksToPrune(5)
		require.NoError(t, err)
		require.Empty(t, rounds)
	})
}

func TestBlockTree_InsertQc(t *testing.T) {
	tree := createTestBlockTree(t)
	require.ErrorContains(t, tree.InsertQc(&drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 2, CurrentRootHash: nil}}), "find block: block for round 2 not found")
	require.ErrorContains(t, tree.InsertQc(&drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 12, CurrentRootHash: test.RandomBytes(32)}}), "qc state hash is different from local computed state hash")
	b, err := tree.FindBlock(12)
	require.NoError(t, err)
	b.RootHash = []byte{1, 2, 3}
	require.NoError(t, tree.InsertQc(&drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 12, CurrentRootHash: []byte{1, 2, 3}}}))
}

func TestNewBlockTree(t *testing.T) {
	bTree := initFromGenesis(t)
	b, err := bTree.FindBlock(1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), b.BlockData.Round)
	require.Nil(t, b.RootHash)
	require.Equal(t, bTree.Root(), b)
	_, err = bTree.FindBlock(2)
	require.Error(t, err)
	require.Len(t, bTree.GetAllUncommittedNodes(), 0)
	require.NotNil(t, bTree.HighQc())
	require.Equal(t, b.CommitQc, bTree.HighQc())
	require.EqualValues(t, 1, bTree.HighQc().GetRound())
}

func TestNewBlockTreeFromDb(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	orchestration := testpartition.NewOrchestration(t, logger.New(t))

	gBlock, err := NewGenesisBlock(orchestration.NetworkID(), crypto.SHA256)
	require.NoError(t, err)
	require.NoError(t, db.Write(blockKey(drctypes.GenesisRootRound), gBlock))

	// create a new block
	block2 := &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:    "test",
			Round:     drctypes.GenesisRootRound + 1,
			Epoch:     0,
			Timestamp: types.NewTimestamp(),
			Payload:   &drctypes.Payload{},
			Qc:        gBlock.Qc,
		},
		HashAlgo: crypto.SHA256,
		RootHash: gBlock.RootHash,
	}
	require.NoError(t, db.Write(blockKey(block2.GetRound()), block2))

	bTree, err := NewBlockTree(db, orchestration)
	require.NoError(t, err)
	require.NotNil(t, bTree)
	require.Len(t, bTree.roundToNode, 2)
	hQc := bTree.HighQc()
	require.NotNil(t, hQc)
	require.Equal(t, uint64(1), hQc.VoteInfo.RoundNumber)
}

func TestNewBlockTreeFromDbChain3Blocks(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	orchestration := testpartition.NewOrchestration(t, logger.New(t))
	gBlock, err := NewGenesisBlock(orchestration.NetworkID(), crypto.SHA256)
	require.NoError(t, err)
	require.NoError(t, db.Write(blockKey(drctypes.GenesisRootRound), gBlock))
	// create blocks 2 and 3
	voteInfoB2 := &drctypes.RoundInfo{
		RoundNumber:       2,
		Timestamp:         types.NewTimestamp(),
		ParentRoundNumber: 1,
		CurrentRootHash:   gBlock.RootHash,
	}
	h2, err := voteInfoB2.Hash(crypto.SHA256)
	require.NoError(t, err)
	qcBlock2 := &drctypes.QuorumCert{
		VoteInfo: voteInfoB2,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:      1,
			PreviousHash: h2,
			Hash:         gBlock.RootHash,
		},
	}
	// create a new block
	block2 := &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:    "test",
			Round:     drctypes.GenesisRootRound + 1,
			Epoch:     0,
			Timestamp: types.NewTimestamp(),
			Payload:   &drctypes.Payload{},
			Qc:        gBlock.Qc,
		},
		HashAlgo: crypto.SHA256,
		RootHash: gBlock.RootHash,
	}
	block3 := &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:    "test",
			Round:     drctypes.GenesisRootRound + 2,
			Epoch:     0,
			Timestamp: types.NewTimestamp() + 1000,
			Payload:   &drctypes.Payload{},
			Qc:        qcBlock2,
		},
		HashAlgo: crypto.SHA256,
		RootHash: gBlock.RootHash,
	}
	require.NoError(t, db.Write(blockKey(block2.BlockData.Round), block2))
	require.NoError(t, db.Write(blockKey(block3.BlockData.Round), block3))

	bTree, err := NewBlockTree(db, orchestration)
	require.NoError(t, err)
	require.NotNil(t, bTree)
	require.Len(t, bTree.roundToNode, 3)
	hQc := bTree.HighQc()
	require.NotNil(t, hQc)
	require.Equal(t, uint64(2), hQc.VoteInfo.RoundNumber)
}

func TestNewBlockTreeFromRecovery(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	gBlock, err := NewGenesisBlock(5, crypto.SHA256)
	require.NoError(t, err)
	require.NoError(t, db.Write(blockKey(drctypes.GenesisRootRound), gBlock))
	// create blocks 2 and 3
	voteInfoB2 := &drctypes.RoundInfo{
		RoundNumber:       2,
		Timestamp:         types.NewTimestamp(),
		ParentRoundNumber: 1,
		CurrentRootHash:   gBlock.RootHash,
	}
	h2, err := voteInfoB2.Hash(crypto.SHA256)
	require.NoError(t, err)
	qcBlock2 := &drctypes.QuorumCert{
		VoteInfo: voteInfoB2,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:      1,
			PreviousHash: h2,
			Hash:         gBlock.RootHash,
		},
	}
	bTree, err := NewBlockTreeWithRootBlock(gBlock, db)
	require.NoError(t, err)
	require.NotNil(t, bTree)
	// create a new block
	block2 := &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:    "test",
			Round:     drctypes.GenesisRootRound + 1,
			Epoch:     0,
			Timestamp: types.NewTimestamp(),
			Payload:   &drctypes.Payload{},
			Qc:        gBlock.Qc,
		},
		HashAlgo: crypto.SHA256,
		RootHash: gBlock.RootHash,
	}
	require.NoError(t, bTree.Add(block2))
	require.NoError(t, bTree.InsertQc(block2.BlockData.Qc))
	block3 := &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:    "test",
			Round:     drctypes.GenesisRootRound + 2,
			Epoch:     0,
			Timestamp: types.NewTimestamp() + 1000,
			Payload:   &drctypes.Payload{},
			Qc:        qcBlock2,
		},
		HashAlgo: crypto.SHA256,
		RootHash: gBlock.RootHash,
	}
	require.NoError(t, bTree.Add(block3))
	require.NoError(t, bTree.InsertQc(block3.BlockData.Qc))
	require.Len(t, bTree.roundToNode, 3)
	hQc := bTree.HighQc()
	require.NotNil(t, hQc)
	require.Equal(t, uint64(2), hQc.VoteInfo.RoundNumber)
}

func TestAddErrorCases(t *testing.T) {
	bTree := initFromGenesis(t)
	//root := bTree.Root()
	// try to add genesis block again
	nextBlock := mockExecutedBlock(drctypes.GenesisRootRound, drctypes.GenesisRootRound, 0)
	require.ErrorContains(t, bTree.Add(nextBlock), "error block for round 1 already exists")
	// unknown parent round i.e. skip a block
	nextBlock = mockExecutedBlock(drctypes.GenesisRootRound+2, drctypes.GenesisRootRound+1, drctypes.GenesisRootRound)
	require.ErrorContains(t, bTree.Add(nextBlock), "error cannot add block for round 3, parent block 2 not found")
	// try to remove root block
	require.ErrorContains(t, bTree.RemoveLeaf(drctypes.GenesisRootRound), "error root cannot be removed")
}

func TestAddAndCommit(t *testing.T) {
	bTree := initFromGenesis(t)
	// append next
	nextBlock := mockExecutedBlock(drctypes.GenesisRootRound+1, drctypes.GenesisRootRound, 0)
	require.NoError(t, bTree.Add(nextBlock))
	// and another
	nextBlock = mockExecutedBlock(drctypes.GenesisRootRound+2, drctypes.GenesisRootRound+1, drctypes.GenesisRootRound)
	require.NoError(t, bTree.Add(nextBlock))
	root := bTree.Root()
	require.Equal(t, root.BlockData.Round, drctypes.GenesisRootRound)
	require.Len(t, bTree.GetAllUncommittedNodes(), 2)
	// find path to root
	blocks := bTree.findPathToRoot(drctypes.GenesisRootRound + 2)
	require.Len(t, blocks, 2)
	require.Equal(t, uint64(3), blocks[0].BlockData.Round)
	require.Equal(t, uint64(2), blocks[1].BlockData.Round)
	pruned, err := bTree.findBlocksToPrune(3)
	require.NoError(t, err)
	require.Len(t, pruned, 2)
	// find path to root
	blocks = bTree.findPathToRoot(drctypes.GenesisRootRound + 1)
	require.Len(t, blocks, 1)
	// find path to root
	blocks = bTree.findPathToRoot(drctypes.GenesisRootRound)
	require.Len(t, blocks, 0)
	require.Len(t, bTree.GetAllUncommittedNodes(), 2)
	require.NoError(t, bTree.RemoveLeaf(drctypes.GenesisRootRound+2))
	require.Len(t, bTree.roundToNode, 2)
	pruned, err = bTree.findBlocksToPrune(2)
	require.NoError(t, err)
	require.Len(t, pruned, 1)
	// current root will be pruned
	require.Equal(t, root.BlockData.Round, pruned[0])
	// add again link qc round is ok, so this is fine
	nextBlock = mockExecutedBlock(drctypes.GenesisRootRound+3, drctypes.GenesisRootRound+1, drctypes.GenesisRootRound)
	require.NoError(t, bTree.Add(nextBlock))
	// make second consecutive round and commit QC for round 4
	nextBlock = mockExecutedBlock(drctypes.GenesisRootRound+4, drctypes.GenesisRootRound+3, drctypes.GenesisRootRound)
	require.NoError(t, bTree.Add(nextBlock))
	// commit block for round 4 which will also commit round 2 (indirectly)
	// test find block
	b, err := bTree.FindBlock(uint64(2))
	require.NoError(t, err)
	require.Equal(t, uint64(2), b.BlockData.Round)
	// test find block
	b, err = bTree.FindBlock(drctypes.GenesisRootRound + 3)
	require.NoError(t, err)
	require.Equal(t, drctypes.GenesisRootRound+3, b.BlockData.Round)
	// set block input data (mock blocks do not have it assigned) as without it
	// commit creates empty unicity tree. As we do not have any shard marked as
	// having changes Commit doesn't return any certificates.
	k := types.PartitionShardID{PartitionID: 1, ShardID: types.ShardID{}.Key()}
	b.ShardState.States[k] = &ShardInfo{PartitionID: 1, IR: &types.InputRecord{}}
	b.RootHash = hexToBytes("F8C1F929F9E718FE5B19DD72BFD23802FFFE5FAC21711BF425548548262942E5")

	commitQc := &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			RoundNumber:       5,
			ParentRoundNumber: 4,
		},
		LedgerCommitInfo: &types.UnicitySeal{
			Version:      1,
			PreviousHash: []byte{1, 2, 3},
			Hash:         b.RootHash,
		},
	}
	certs, err := bTree.Commit(commitQc)
	require.NoError(t, err)
	require.Empty(t, certs)
	newRoot := bTree.Root()
	require.Equal(t, newRoot, b)
	require.Len(t, bTree.roundToNode, 2)
	require.Len(t, bTree.GetAllUncommittedNodes(), 1)
}
