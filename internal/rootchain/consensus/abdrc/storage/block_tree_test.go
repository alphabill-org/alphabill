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

func initFromGenesis(t *testing.T) *BlockTree {
	t.Helper()
	gBlock := NewExecutedBlockFromGenesis(gocrypto.SHA256, pg)
	db := memorydb.New()
	require.NoError(t, blockStoreGenesisInit(gBlock, db))
	btree, err := NewBlockTree(db)
	require.NoError(t, err)
	return btree
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
