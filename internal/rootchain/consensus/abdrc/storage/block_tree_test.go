package storage

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var sysID1 = types.SystemID32(1)
var sysID2 = types.SystemID32(2)

var zeroHash = make([]byte, gocrypto.SHA256.Size())

var inputRecord1 = &types.InputRecord{
	PreviousHash: []byte{1, 1, 1},
	Hash:         []byte{2, 2, 2},
	BlockHash:    []byte{3, 3, 3},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  1,
}
var sdr1 = &genesis.SystemDescriptionRecord{
	SystemIdentifier: sysID1.ToSystemID(),
	T2Timeout:        2500,
}
var inputRecord2 = &types.InputRecord{
	PreviousHash: []byte{1, 1, 1},
	Hash:         []byte{5, 5, 5},
	BlockHash:    []byte{3, 3, 3},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  1,
}
var sdr2 = &genesis.SystemDescriptionRecord{
	SystemIdentifier: sysID2.ToSystemID(),
	T2Timeout:        2500,
}

var roundInfo = &abtypes.RoundInfo{
	RoundNumber:     genesis.RootRound,
	Timestamp:       genesis.Timestamp,
	CurrentRootHash: []byte{0x70, 0x0D, 0x99, 0x66, 0xD5, 0x0F, 0x04, 0xE0, 0x44, 0xB2, 0xDB, 0x3F, 0x7B, 0x28, 0xB3, 0x3D, 0x6C, 0xC1, 0xD2, 0xB1, 0x65, 0xCB, 0xC7, 0x5C, 0xE3, 0xA3, 0x6D, 0xAF, 0xB4, 0xC9, 0x29, 0x3A},
}
var pg = []*genesis.GenesisPartitionRecord{
	{
		Certificate: &types.UnicityCertificate{
			InputRecord: inputRecord1,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				SystemIdentifier:      sysID1.ToSystemID(),
				SystemDescriptionHash: sdr1.Hash(gocrypto.SHA256),
			},
			UnicitySeal: &types.UnicitySeal{
				RootChainRoundNumber: roundInfo.RoundNumber,
				Hash:                 roundInfo.CurrentRootHash,
				Timestamp:            roundInfo.Timestamp,
				PreviousHash:         roundInfo.Hash(gocrypto.SHA256),
				Signatures:           map[string][]byte{},
			},
		},
		SystemDescriptionRecord: sdr1,
	},
	{
		Certificate: &types.UnicityCertificate{
			InputRecord: inputRecord2,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				SystemIdentifier:      sysID2.ToSystemID(),
				SystemDescriptionHash: sdr2.Hash(gocrypto.SHA256),
			},
			UnicitySeal: &types.UnicitySeal{
				RootChainRoundNumber: roundInfo.RoundNumber,
				Hash:                 []byte{0x0A, 0x51, 0x80, 0x59, 0x3D, 0x0C, 0xAB, 0x52, 0xA3, 0xAD, 0xFF, 0x2B, 0xDE, 0x8A, 0x11, 0xE3, 0x10, 0xED, 0xB6, 0xFD, 0xA3, 0x26, 0x0E, 0xBF, 0x45, 0x0B, 0x1D, 0x9B, 0xFB, 0xA9, 0x9B, 0x9A},
				Timestamp:            roundInfo.Timestamp,
				PreviousHash:         roundInfo.Hash(gocrypto.SHA256),
				Signatures:           map[string][]byte{},
			},
		},
		SystemDescriptionRecord: sdr2,
	},
}

func mockExecutedBlock(round, qcRound, qcParentRound uint64) *ExecutedBlock {
	return &ExecutedBlock{
		BlockData: &abtypes.BlockData{
			Round: round,
			Qc: &abtypes.QuorumCert{
				VoteInfo: &abtypes.RoundInfo{
					RoundNumber:       qcRound,
					ParentRoundNumber: qcParentRound,
					Epoch:             0,
					CurrentRootHash:   zeroHash,
				},
				LedgerCommitInfo: &types.UnicitySeal{
					Hash: zeroHash,
				},
			},
		},
	}
}

// createTestBlockTree creates the following tree
/*
   ╭--> B6--> B7--> B8
  B5--> B9--> B10
         ╰--> B11--> B12
*/
func createTestBlockTree(t *testing.T) *BlockTree {
	treeNodes := make(map[uint64]*node)
	db := memorydb.New()
	rootNode := &node{data: &ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B5", Round: 5}}}
	b6 := newNode(&ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B6", Round: 6, Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 5}}}})
	b7 := newNode(&ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B7", Round: 7, Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 6}}}})
	b8 := newNode(&ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B8", Round: 8, Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 7}}}})
	b9 := newNode(&ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B9", Round: 9, Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 5}}}})
	b10 := newNode(&ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B10", Round: 10, Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 9}}}})
	b11 := newNode(&ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B11", Round: 11, Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 9}}}})
	b12 := newNode(&ExecutedBlock{BlockData: &abtypes.BlockData{Author: "B12", Round: 12, Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 11}}}})
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
	gBlock := NewExecutedBlockFromGenesis(gocrypto.SHA256, pg)
	db := memorydb.New()
	require.NoError(t, blockStoreGenesisInit(gBlock, db))
	btree, err := NewBlockTree(db)
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
	blocks := tree.FindPathToRoot(8)
	require.Len(t, blocks, 3)
	// B8-->B7-->B6-->root (not included)
	require.Equal(t, "B8", blocks[0].BlockData.Author)
	require.Equal(t, "B7", blocks[1].BlockData.Author)
	require.Equal(t, "B6", blocks[2].BlockData.Author)
	blocks = tree.FindPathToRoot(10)
	// B10-->B9-->root (not included)
	require.Len(t, blocks, 2)
	require.Equal(t, "B10", blocks[0].BlockData.Author)
	require.Equal(t, "B9", blocks[1].BlockData.Author)
	blocks = tree.FindPathToRoot(12)
	// B12-->B11-->B9-->root (not included)
	require.Len(t, blocks, 3)
	require.Equal(t, "B12", blocks[0].BlockData.Author)
	require.Equal(t, "B11", blocks[1].BlockData.Author)
	require.Equal(t, "B9", blocks[2].BlockData.Author)
	// node not found
	require.Nil(t, tree.FindPathToRoot(2))
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
	require.ErrorContains(t, tree.InsertQc(&abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 2, CurrentRootHash: zeroHash}}), "block tree add qc failed, block for round 2 not found")
	require.ErrorContains(t, tree.InsertQc(&abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 12, CurrentRootHash: zeroHash}}), "block tree add qc failed, qc state hash is different from local computed state hash")
	b, err := tree.FindBlock(12)
	require.NoError(t, err)
	b.RootHash = []byte{1, 2, 3}
	require.NoError(t, tree.InsertQc(&abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 12, CurrentRootHash: []byte{1, 2, 3}}}))
}

func TestNewBlockTree(t *testing.T) {
	bTree := initFromGenesis(t)
	b, err := bTree.FindBlock(1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), b.BlockData.Round)
	require.Equal(t, pg[0].Certificate.UnicitySeal.Hash, b.RootHash)
	require.Equal(t, bTree.Root(), b)
	_, err = bTree.FindBlock(2)
	require.Error(t, err)
	require.Len(t, bTree.GetAllUncommittedNodes(), 0)
	require.Equal(t, b.CommitQc, bTree.HighQc())
}

func TestNewBlockTreeFromDb(t *testing.T) {
	db := memorydb.New()
	gBlock := NewExecutedBlockFromGenesis(gocrypto.SHA256, pg)
	require.NoError(t, db.Write(blockKey(genesis.RootRound), gBlock))
	// create a new block
	block2 := &ExecutedBlock{
		BlockData: &abtypes.BlockData{
			Author:    "test",
			Round:     genesis.RootRound + 1,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   &abtypes.Payload{},
			Qc:        gBlock.Qc,
		},
		CurrentIR: gBlock.CurrentIR,
		Changed:   make([]types.SystemID32, 0),
		HashAlgo:  gocrypto.SHA256,
		RootHash:  gBlock.Qc.LedgerCommitInfo.Hash,
		Qc:        nil, // qc to genesis
		CommitQc:  nil, // does not commit a block
	}
	require.NoError(t, db.Write(blockKey(block2.BlockData.Round), block2))
	bTree, err := NewBlockTree(db)
	require.NoError(t, err)
	require.NotNil(t, bTree)
	require.Len(t, bTree.roundToNode, 2)
	hQc := bTree.HighQc()
	require.NotNil(t, hQc)
	require.Equal(t, uint64(1), hQc.VoteInfo.RoundNumber)
}

func TestNewBlockTreeFromDbChain3Blocks(t *testing.T) {
	db := memorydb.New()

	gBlock := NewExecutedBlockFromGenesis(gocrypto.SHA256, pg)
	require.NoError(t, db.Write(blockKey(genesis.RootRound), gBlock))
	// create blocks 2 and 3
	voteInfoB2 := &abtypes.RoundInfo{
		RoundNumber:       2,
		Timestamp:         util.MakeTimestamp(),
		ParentRoundNumber: 1,
		CurrentRootHash:   gBlock.RootHash,
	}
	qcBlock2 := &abtypes.QuorumCert{
		VoteInfo: voteInfoB2,
		LedgerCommitInfo: &types.UnicitySeal{
			PreviousHash: voteInfoB2.Hash(gocrypto.SHA256),
			Hash:         gBlock.RootHash,
		},
	}
	// create a new block
	block2 := &ExecutedBlock{
		BlockData: &abtypes.BlockData{
			Author:    "test",
			Round:     genesis.RootRound + 1,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   &abtypes.Payload{},
			Qc:        gBlock.Qc,
		},
		CurrentIR: gBlock.CurrentIR,
		Changed:   make([]types.SystemID32, 0),
		HashAlgo:  gocrypto.SHA256,
		RootHash:  gBlock.Qc.LedgerCommitInfo.Hash,
		Qc:        qcBlock2,
		CommitQc:  qcBlock2,
	}
	block3 := &ExecutedBlock{
		BlockData: &abtypes.BlockData{
			Author:    "test",
			Round:     genesis.RootRound + 2,
			Epoch:     0,
			Timestamp: util.MakeTimestamp() + 1000,
			Payload:   &abtypes.Payload{},
			Qc:        qcBlock2,
		},
		CurrentIR: gBlock.CurrentIR,
		Changed:   make([]types.SystemID32, 0),
		HashAlgo:  gocrypto.SHA256,
		RootHash:  gBlock.Qc.LedgerCommitInfo.Hash,
	}
	require.NoError(t, db.Write(blockKey(block2.BlockData.Round), block2))
	require.NoError(t, db.Write(blockKey(block3.BlockData.Round), block3))

	bTree, err := NewBlockTree(db)
	require.NoError(t, err)
	require.NotNil(t, bTree)
	require.Len(t, bTree.roundToNode, 3)
	hQc := bTree.HighQc()
	require.NotNil(t, hQc)
	require.Equal(t, uint64(2), hQc.VoteInfo.RoundNumber)
}

func TestNewBlockTreeFromRecovery(t *testing.T) {
	db := memorydb.New()
	gBlock := NewExecutedBlockFromGenesis(gocrypto.SHA256, pg)
	require.NoError(t, db.Write(blockKey(genesis.RootRound), gBlock))
	// create blocks 2 and 3
	voteInfoB2 := &abtypes.RoundInfo{
		RoundNumber:       2,
		Timestamp:         util.MakeTimestamp(),
		ParentRoundNumber: 1,
		CurrentRootHash:   gBlock.RootHash,
	}
	qcBlock2 := &abtypes.QuorumCert{
		VoteInfo: voteInfoB2,
		LedgerCommitInfo: &types.UnicitySeal{
			PreviousHash: voteInfoB2.Hash(gocrypto.SHA256),
			Hash:         gBlock.RootHash,
		},
	}
	// create a new block
	block2 := &ExecutedBlock{
		BlockData: &abtypes.BlockData{
			Author:    "test",
			Round:     genesis.RootRound + 1,
			Epoch:     0,
			Timestamp: util.MakeTimestamp(),
			Payload:   &abtypes.Payload{},
			Qc:        gBlock.Qc,
		},
		CurrentIR: gBlock.CurrentIR,
		Changed:   make([]types.SystemID32, 0),
		HashAlgo:  gocrypto.SHA256,
		RootHash:  gBlock.Qc.LedgerCommitInfo.Hash,
		Qc:        qcBlock2,
		CommitQc:  qcBlock2,
	}
	block3 := &ExecutedBlock{
		BlockData: &abtypes.BlockData{
			Author:    "test",
			Round:     genesis.RootRound + 2,
			Epoch:     0,
			Timestamp: util.MakeTimestamp() + 1000,
			Payload:   &abtypes.Payload{},
			Qc:        qcBlock2,
		},
		CurrentIR: gBlock.CurrentIR,
		Changed:   make([]types.SystemID32, 0),
		HashAlgo:  gocrypto.SHA256,
		RootHash:  gBlock.Qc.LedgerCommitInfo.Hash,
	}
	bTree, err := NewBlockTreeFromRecovery(gBlock, []*ExecutedBlock{block2, block3}, db)
	require.NoError(t, err)
	require.NotNil(t, bTree)
	require.Len(t, bTree.roundToNode, 3)
	hQc := bTree.HighQc()
	require.NotNil(t, hQc)
	require.Equal(t, uint64(2), hQc.VoteInfo.RoundNumber)
}

func TestAddErrorCases(t *testing.T) {
	bTree := initFromGenesis(t)
	//root := bTree.Root()
	// try to add genesis block again
	nextBlock := mockExecutedBlock(genesis.RootRound, genesis.RootRound, 0)
	require.ErrorContains(t, bTree.Add(nextBlock), "error block for round 1 already exists")
	// unknown parent round i.e. skip a block
	nextBlock = mockExecutedBlock(genesis.RootRound+2, genesis.RootRound+1, genesis.RootRound)
	require.ErrorContains(t, bTree.Add(nextBlock), "error cannot add block for round 3, parent block 2 not found")
	// try to remove root block
	require.ErrorContains(t, bTree.RemoveLeaf(genesis.RootRound), "error root cannot be removed")
}

func TestAddAndCommit(t *testing.T) {
	bTree := initFromGenesis(t)
	// append next
	nextBlock := mockExecutedBlock(genesis.RootRound+1, genesis.RootRound, 0)
	require.NoError(t, bTree.Add(nextBlock))
	// and another
	nextBlock = mockExecutedBlock(genesis.RootRound+2, genesis.RootRound+1, genesis.RootRound)
	require.NoError(t, bTree.Add(nextBlock))
	root := bTree.Root()
	require.Equal(t, root.BlockData.Round, genesis.RootRound)
	require.Len(t, bTree.GetAllUncommittedNodes(), 2)
	// find path to root
	blocks := bTree.FindPathToRoot(genesis.RootRound + 2)
	require.Len(t, blocks, 2)
	require.Equal(t, uint64(3), blocks[0].BlockData.Round)
	require.Equal(t, uint64(2), blocks[1].BlockData.Round)
	pruned, err := bTree.findBlocksToPrune(3)
	require.NoError(t, err)
	require.Len(t, pruned, 2)
	// find path to root
	blocks = bTree.FindPathToRoot(genesis.RootRound + 1)
	require.Len(t, blocks, 1)
	// find path to root
	blocks = bTree.FindPathToRoot(genesis.RootRound)
	require.Len(t, blocks, 0)
	require.Len(t, bTree.GetAllUncommittedNodes(), 2)
	require.NoError(t, bTree.RemoveLeaf(genesis.RootRound+2))
	require.Len(t, bTree.roundToNode, 2)
	pruned, err = bTree.findBlocksToPrune(2)
	require.NoError(t, err)
	require.Len(t, pruned, 1)
	// current root will be pruned
	require.Equal(t, root.BlockData.Round, pruned[0])
	// add again link qc round is ok, so this is fine
	nextBlock = mockExecutedBlock(genesis.RootRound+3, genesis.RootRound+1, genesis.RootRound)
	require.NoError(t, bTree.Add(nextBlock))
	// make second consecutive round and commit QC for round 4
	nextBlock = mockExecutedBlock(genesis.RootRound+4, genesis.RootRound+3, genesis.RootRound)
	require.NoError(t, bTree.Add(nextBlock))
	// commit block for round 4 which will also commit round 2 (indirectly)
	// test find block
	b, err := bTree.FindBlock(uint64(2))
	require.NoError(t, err)
	require.Equal(t, uint64(2), b.BlockData.Round)
	// test find block
	b, err = bTree.FindBlock(genesis.RootRound + 3)
	require.NoError(t, err)
	require.Equal(t, genesis.RootRound+3, b.BlockData.Round)

	commitQc := &abtypes.QuorumCert{
		VoteInfo: &abtypes.RoundInfo{
			RoundNumber:       5,
			ParentRoundNumber: 4,
		},
		LedgerCommitInfo: &types.UnicitySeal{
			PreviousHash: []byte{1, 2, 3},
			Hash:         zeroHash,
		},
	}
	cBlock, err := bTree.Commit(commitQc)
	require.NoError(t, err)
	require.NotNil(t, cBlock)
	newRoot := bTree.Root()
	require.Equal(t, newRoot, b)
	require.Len(t, bTree.roundToNode, 2)
	require.Len(t, bTree.GetAllUncommittedNodes(), 1)
}
