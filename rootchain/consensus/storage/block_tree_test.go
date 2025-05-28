package storage

import (
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

func mockExecutedBlock(round, qcRound uint64) ExecutedBlock {
	var randomHash = test.RandomBytes(32)
	return ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Round: round,
			Qc: &drctypes.QuorumCert{
				VoteInfo: &drctypes.RoundInfo{
					RoundNumber:       qcRound,
					ParentRoundNumber: qcRound - 1,
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
	// we actually do not need the storage for (all) these tests!?
	db, err := NewBoltStorage(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	treeNodes := make(map[uint64]*node)
	rootNode := &node{data: &ExecutedBlock{BlockData: &drctypes.BlockData{Author: "B5", Round: 5}, CommitQc: &drctypes.QuorumCert{}}}
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
	require.NoError(t, db.WriteBlock(rootNode.data, true))
	treeNodes[6] = b6
	require.NoError(t, db.WriteBlock(b6.data, false))
	treeNodes[7] = b7
	require.NoError(t, db.WriteBlock(b7.data, false))
	treeNodes[8] = b8
	require.NoError(t, db.WriteBlock(b8.data, false))
	treeNodes[9] = b9
	require.NoError(t, db.WriteBlock(b9.data, false))
	treeNodes[10] = b10
	require.NoError(t, db.WriteBlock(b10.data, false))
	treeNodes[11] = b11
	require.NoError(t, db.WriteBlock(b11.data, false))
	treeNodes[12] = b12
	require.NoError(t, db.WriteBlock(b12.data, false))

	return &BlockTree{
		root:        rootNode,
		roundToNode: treeNodes,
		blocksDB:    db,
	}
}

func TestBlockTree_RemoveLeaf(t *testing.T) {
	tree := createTestBlockTree(t)
	// remove leaf that has children
	require.ErrorContains(t, tree.RemoveLeaf(9), "error round 9 is not leaf node")
	require.ErrorContains(t, tree.RemoveLeaf(5), "root block cannot be removed")
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

func Test_NewBlockTree(t *testing.T) {
	// in most tests it's OK to return no PDRs from Orchestration
	orchestration := mockOrchestration{
		shardConfigs: func(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
			return nil, nil
		},
	}
	// prepare block chain with committed/root and two uncommitted child blocks.
	blockRoot := ExecutedBlock{BlockData: &drctypes.BlockData{Round: 4}, CommitQc: &drctypes.QuorumCert{}}
	block1 := mockExecutedBlock(blockRoot.GetRound()+1, blockRoot.GetRound())
	block2 := mockExecutedBlock(block1.GetRound()+1, block1.GetRound())

	t.Run("invalid argument", func(t *testing.T) {
		bt, err := NewBlockTree(nil, nil)
		require.EqualError(t, err, `block tree init failed, database is nil`)
		require.Nil(t, bt)
	})

	t.Run("load blocks fails", func(t *testing.T) {
		expErr := errors.New("failure to load blocks")
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return nil, expErr },
		}
		bt, err := NewBlockTree(db, nil)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, bt)
	})

	t.Run("no root block", func(t *testing.T) {
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&block2, &block1}, nil },
		}
		bt, err := NewBlockTree(db, nil)
		require.EqualError(t, err, `root block not found`)
		require.Nil(t, bt)
	})

	t.Run("invalid blocks - gap", func(t *testing.T) {
		db := mockPersistentStore{
			// missing block between block2 and root
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&block2, &blockRoot}, nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.EqualError(t, err, `cannot add block for round 6, parent block 5 not found`)
		require.Nil(t, bt)
	})

	t.Run("failure to init root block", func(t *testing.T) {
		expErr := errors.New("no ShardInfo")
		orchestration := mockOrchestration{
			shardConfigs: func(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
				if rootRound == blockRoot.GetRound() {
					return nil, expErr
				}
				return nil, nil
			},
		}
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&blockRoot}, nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, bt)
	})

	t.Run("failure to init child block", func(t *testing.T) {
		expErr := errors.New("no ShardInfo")
		orchestration := mockOrchestration{
			shardConfigs: func(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
				if rootRound == block1.GetRound() {
					return nil, expErr
				}
				return nil, nil
			},
		}
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&block1, &blockRoot}, nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, bt)
	})

	t.Run("multiple root candidates", func(t *testing.T) {
		root2 := ExecutedBlock{BlockData: &drctypes.BlockData{Round: block1.GetRound() + 1}, CommitQc: &drctypes.QuorumCert{}}
		db := mockPersistentStore{
			// both root2 and blockRoot are committed nodes, first one (the persistent store is expected to return
			// nodes in the descending round order!) is selected to be the root (root2 in this case)
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&root2, &block1, &blockRoot}, nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.NoError(t, err)
		require.NotNil(t, bt)
		require.Equal(t, &root2, bt.Root())
	})

	t.Run("build tree from existing state", func(t *testing.T) {
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&block2, &block1, &blockRoot}, nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.NoError(t, err)
		require.NotNil(t, bt)
		require.Equal(t, &blockRoot, bt.Root())
		b, err := bt.FindBlock(block1.GetRound())
		require.NoError(t, err)
		require.Equal(t, &block1, b)
		b, err = bt.FindBlock(block2.GetRound())
		require.NoError(t, err)
		require.Equal(t, &block2, b)
		blocks := bt.GetAllUncommittedNodes()
		if assert.Len(t, blocks, 2) {
			require.Contains(t, blocks, &block1)
			require.Contains(t, blocks, &block2)
		}
	})

	t.Run("no blocks, init from genesis", func(t *testing.T) {
		db := mockPersistentStore{
			// returning no blocks/no error triggers initialization with genesis
			loadBlocks: func() ([]*ExecutedBlock, error) { return nil, nil },
			// the genesis block is stored to DB
			writeBlock: func(block *ExecutedBlock, root bool) error {
				require.Equal(t, drctypes.GenesisRootRound, block.GetRound())
				return nil
			},
		}
		bt, err := NewBlockTree(db, orchestration)
		require.NoError(t, err)
		require.NotNil(t, bt)
		rootB := bt.Root()
		require.NotNil(t, rootB)
		require.Equal(t, drctypes.GenesisRootRound, rootB.GetRound())
		require.Len(t, bt.GetAllUncommittedNodes(), 0)
		require.NotNil(t, bt.HighQc())
	})
}

func Test_NewBlockTreeWithRootBlock(t *testing.T) {
	t.Run("db storage failure", func(t *testing.T) {
		expErr := errors.New("can't store the block")
		db := mockPersistentStore{
			writeBlock: func(block *ExecutedBlock, root bool) error { return expErr },
		}
		bt, err := NewBlockTreeWithRootBlock(&ExecutedBlock{}, db)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, bt)
	})

	t.Run("success", func(t *testing.T) {
		recoveryBlock := mockExecutedBlock(990, 989)
		stored := false
		db := mockPersistentStore{
			writeBlock: func(block *ExecutedBlock, root bool) error {
				require.Equal(t, &recoveryBlock, block)
				require.True(t, root)
				stored = true
				return nil
			},
		}
		bt, err := NewBlockTreeWithRootBlock(&recoveryBlock, db)
		require.NoError(t, err)
		require.NotNil(t, bt)
		require.True(t, stored)
	})
}

func Test_BlockTree_Add(t *testing.T) {
	// in most tests it's OK to return no PDRs from Orchestration - the ShardInfo
	// of the blocks will not be initialized but it's not used by the test
	orchestration := mockOrchestration{
		shardConfigs: func(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
			return nil, nil
		},
	}
	// prepare block chain with committed/root and two uncommitted child blocks.
	blockRoot := ExecutedBlock{BlockData: &drctypes.BlockData{Round: 4}, CommitQc: &drctypes.QuorumCert{}}
	block1 := mockExecutedBlock(blockRoot.GetRound()+1, blockRoot.GetRound())
	block2 := mockExecutedBlock(block1.GetRound()+1, block1.GetRound())

	t.Run("round already exists", func(t *testing.T) {
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&block2, &block1, &blockRoot}, nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.NoError(t, err)
		require.EqualError(t, bt.Add(&blockRoot), `block for round 4 already exists`)
		require.EqualError(t, bt.Add(&block1), `block for round 5 already exists`)
		require.EqualError(t, bt.Add(&block2), `block for round 6 already exists`)
		// blocks should still be in the tree
		b, err := bt.FindBlock(block2.GetRound())
		require.NoError(t, err)
		require.Equal(t, &block2, b)
	})

	t.Run("no parent round", func(t *testing.T) {
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&blockRoot}, nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.NoError(t, err)
		require.EqualError(t, bt.Add(&block2), `cannot add block for round 6, parent block 5 not found`)
		// make sure the new block is not in the tree
		b, err := bt.FindBlock(block2.GetRound())
		require.EqualError(t, err, `block for round 6 not found`)
		require.Nil(t, b)
	})

	t.Run("persisting new block fails", func(t *testing.T) {
		expErr := errors.New("storage failure")
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&block1, &blockRoot}, nil },
			writeBlock: func(block *ExecutedBlock, root bool) error { return expErr },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.NoError(t, err)
		require.ErrorIs(t, bt.Add(&block2), expErr)
		// persisting is the last op so the block is in the memory tree!
		b, err := bt.FindBlock(block2.GetRound())
		require.NoError(t, err)
		require.Equal(t, &block2, b)
	})

	t.Run("success", func(t *testing.T) {
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&blockRoot}, nil },
			writeBlock: func(block *ExecutedBlock, root bool) error { return nil },
		}
		bt, err := NewBlockTree(db, orchestration)
		require.NoError(t, err)
		require.Len(t, bt.GetAllUncommittedNodes(), 0)

		require.NoError(t, bt.Add(&block1))
		require.Len(t, bt.GetAllUncommittedNodes(), 1)

		require.NoError(t, bt.Add(&block2))
		b, err := bt.FindBlock(block2.GetRound())
		require.NoError(t, err)
		require.Equal(t, &block2, b)
		require.Len(t, bt.GetAllUncommittedNodes(), 2)

		require.Equal(t, &blockRoot, bt.Root(), "root should not have changed")

		// set block input data (mock blocks do not have it assigned) as without it
		// commit creates empty unicity tree. As we do not have any shard marked as
		// having changes Commit doesn't return any certificates.
		b, err = bt.FindBlock(block1.GetRound())
		require.NoError(t, err)
		k := types.PartitionShardID{PartitionID: 1, ShardID: types.ShardID{}.Key()}
		b.ShardState.States[k] = &ShardInfo{PartitionID: 1, IR: &types.InputRecord{}}
		b.RootHash = hexToBytes("F8C1F929F9E718FE5B19DD72BFD23802FFFE5FAC21711BF425548548262942E5")

		commitQc := &drctypes.QuorumCert{
			VoteInfo: &drctypes.RoundInfo{
				RoundNumber:       b.GetRound() + 1,
				ParentRoundNumber: b.GetRound(),
			},
			LedgerCommitInfo: &types.UnicitySeal{
				Version:      1,
				PreviousHash: []byte{1, 2, 3},
				Hash:         b.RootHash,
			},
		}
		certs, err := bt.Commit(commitQc)
		require.NoError(t, err)
		require.Empty(t, certs)
		newRoot := bt.Root()
		require.Equal(t, newRoot, b)
		require.Len(t, bt.roundToNode, 2)
		require.Len(t, bt.GetAllUncommittedNodes(), 1)
	})
}
