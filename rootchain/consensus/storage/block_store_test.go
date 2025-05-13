package storage

import (
	"crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	testpartition "github.com/alphabill-org/alphabill/rootchain/partitions/testutils"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
)

func initBlockStoreFromGenesis(t *testing.T, shardConf *types.PartitionDescriptionRecord) *BlockStore {
	t.Helper()

	db, err := memorydb.New()
	require.NoError(t, err)

	if shardConf != nil {
		genesisBlock := genesisBlockWithShard(t, shardConf)
		require.NoError(t, WriteBlock(db, genesisBlock))
	}

	log := logger.New(t)
	dir := t.TempDir()
	orchestration, err := partitions.NewOrchestration(5, filepath.Join(dir, "orchestration.db"), log)
	require.NoError(t, err)
	if shardConf != nil {
		require.NoError(t, orchestration.AddShardConfig(shardConf))
	}
	t.Cleanup(func() { _ = orchestration.Close() })

	bStore, err := New(crypto.SHA256, db, orchestration, logger.New(t))
	require.NoError(t, err)
	return bStore
}

func TestNewBlockStoreFromGenesis(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t, nil)
	hQc := bStore.GetHighQc()
	require.Equal(t, uint64(1), hQc.VoteInfo.RoundNumber)
	require.Nil(t, bStore.IsChangeInProgress(1, types.ShardID{}))
	b, err := bStore.Block(1)
	require.NoError(t, err)
	require.Nil(t, b.RootHash)
	_, err = bStore.Block(2)
	require.ErrorContains(t, err, "block for round 2 not found")
	require.Len(t, bStore.GetCertificates(), 0)
}

func fakeBlock(round uint64, qc *drctypes.QuorumCert) *ExecutedBlock {
	return &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:  "test",
			Round:   round,
			Payload: &drctypes.Payload{},
			Qc:      qc,
		},
		HashAlgo:   crypto.SHA256,
		RootHash:   make([]byte, 32),
		Qc:         &drctypes.QuorumCert{},
		CommitQc:   nil,
		ShardState: ShardStates{},
	}
}

func TestNewBlockStoreFromDB_MultipleRoots(t *testing.T) {
	orchestration := testpartition.NewOrchestration(t, logger.New(t))
	db, err := memorydb.New()
	require.NoError(t, err)
	require.NoError(t, storeGenesisInit(db, 5, crypto.SHA256))
	// create second root
	vInfo9 := &drctypes.RoundInfo{RoundNumber: 9, ParentRoundNumber: 8}
	h9, err := vInfo9.Hash(crypto.SHA256)
	require.NoError(t, err)
	b10 := fakeBlock(10, &drctypes.QuorumCert{
		VoteInfo: vInfo9,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:      1,
			PreviousHash: h9,
			Hash:         test.RandomBytes(32),
		},
	})
	require.NoError(t, db.Write(blockKey(b10.GetRound()), b10))

	vInfo8 := &drctypes.RoundInfo{RoundNumber: 8, ParentRoundNumber: 7}
	h8, err := vInfo8.Hash(crypto.SHA256)
	require.NoError(t, err)
	b9 := fakeBlock(9, &drctypes.QuorumCert{
		VoteInfo: vInfo8,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			PreviousHash:         h8,
			RootChainRoundNumber: 8,
			Hash:                 test.RandomBytes(32),
		},
	})
	b9.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}
	require.NoError(t, db.Write(blockKey(b9.GetRound()), b9))

	vInfo7 := &drctypes.RoundInfo{RoundNumber: 7, ParentRoundNumber: 6}
	h7, err := vInfo7.Hash(crypto.SHA256)
	require.NoError(t, err)
	b8 := fakeBlock(8, &drctypes.QuorumCert{
		VoteInfo: vInfo7,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			PreviousHash:         h7,
			RootChainRoundNumber: 7,
			Hash:                 test.RandomBytes(32),
		},
	})
	b8.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 8}}
	b8.CommitQc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}
	require.NoError(t, db.Write(blockKey(b8.GetRound()), b8))
	// load from DB
	bStore, err := New(crypto.SHA256, db, orchestration, logger.New(t))
	require.NoError(t, err)
	// although store contains more than one root, the latest is preferred
	require.EqualValues(t, 8, bStore.blockTree.Root().GetRound())
	// first root is cleaned up
	itr := db.Find([]byte(blockPrefix))
	defer func() { require.NoError(t, itr.Close()) }()
	i := 0
	// the db now contains blocks 8,9,10 the old blocks have been cleaned up
	for ; itr.Valid() && strings.HasPrefix(string(itr.Key()), blockPrefix); itr.Next() {
		var b ExecutedBlock
		require.NoError(t, itr.Value(&b))
		require.EqualValues(t, 8+i, b.GetRound())
		i++
	}
	require.EqualValues(t, 3, i)

}

func TestNewBlockStoreFromDB_InvalidDBContainsCap(t *testing.T) {
	orchestration := testpartition.NewOrchestration(t, logger.New(t))
	db, err := memorydb.New()
	require.NoError(t, err)
	require.NoError(t, storeGenesisInit(db, 5, crypto.SHA256))
	// create a second chain, that has no root
	b10 := fakeBlock(10, &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}})
	require.NoError(t, db.Write(blockKey(b10.GetRound()), b10))
	b9 := fakeBlock(9, &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 8}})
	b9.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}
	require.NoError(t, db.Write(blockKey(b9.GetRound()), b9))
	b8 := fakeBlock(8, &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}})
	b8.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 8}}
	b8.CommitQc = nil
	require.NoError(t, db.Write(blockKey(b8.GetRound()), b8))
	// load from DB
	bStore, err := New(crypto.SHA256, db, orchestration, logger.New(t))
	require.ErrorContains(t, err, `initializing block tree: cannot add block for round 8, parent block 7 not found`)
	require.Nil(t, bStore)
}

func TestNewBlockStoreFromDB_NoRootBlock(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	// create a chain that has not got a root block
	b10 := fakeBlock(10, &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}})
	require.NoError(t, db.Write(blockKey(b10.GetRound()), b10))
	b9 := fakeBlock(9, &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 8}})
	b9.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}
	require.NoError(t, db.Write(blockKey(b9.GetRound()), b9))
	b8 := fakeBlock(8, &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}})
	b8.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 8}}
	b8.CommitQc = nil
	require.NoError(t, db.Write(blockKey(b8.GetRound()), b8))
	// load from DB
	log := logger.New(t)
	bStore, err := New(crypto.SHA256, db, testpartition.NewOrchestration(t, log), log)
	require.ErrorContains(t, err, `initializing block tree: root block not found`)
	require.Nil(t, bStore)
}

func TestHandleTcError(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t, nil)
	require.ErrorContains(t, bStore.ProcessTc(nil), "tc is nil")
	tc := &drctypes.TimeoutCert{
		Timeout: &drctypes.Timeout{
			Round: 3,
		},
	}
	// For now ignore if we do not have a block that timeouts
	require.NoError(t, bStore.ProcessTc(tc))
}

func TestHandleQcError(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t, nil)
	ucs, err := bStore.ProcessQc(nil)
	require.ErrorContains(t, err, "qc is nil")
	require.Nil(t, ucs)
	// qc for non-existing round
	qc := &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
		},
	}
	ucs, err = bStore.ProcessQc(qc)
	require.ErrorContains(t, err, "block for round 3 not found")
	require.Nil(t, ucs)
}

func TestBlockStoreAdd(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t, nil)
	rBlock := bStore.blockTree.Root()
	mockBlockVer := mockIRVerifier{
		verify: func(round uint64, irChReq *drctypes.IRChangeReq) (*types.InputRecord, error) {
			luc, err := bStore.GetCertificate(irChReq.Partition, irChReq.Shard)
			if err != nil {
				return nil, fmt.Errorf("invalid payload: partition %s state is missing: %w", irChReq.Partition, err)
			}
			switch irChReq.CertReason {
			case drctypes.Quorum:
				// NB! there was at least one request, otherwise we would not be here
				return irChReq.Requests[0].InputRecord, nil
			case drctypes.QuorumNotPossible:
			case drctypes.T2Timeout:
				return luc.UC.InputRecord, nil
			}
			return nil, fmt.Errorf("unknown certification reason %v", irChReq.CertReason)
		},
	}
	block := &drctypes.BlockData{
		Round:   drctypes.GenesisRootRound,
		Payload: &drctypes.Payload{},
		Qc:      nil,
	}
	_, err := bStore.Add(block, mockBlockVer)
	require.ErrorContains(t, err, "add block failed: different block for round 1 is already in store")
	block = &drctypes.BlockData{
		Round:   drctypes.GenesisRootRound + 1,
		Payload: &drctypes.Payload{},
		Qc:      bStore.GetHighQc(), // the qc is genesis qc
	}
	// Proposal always comes with Qc and Block, process Qc first and then new block
	// add stale qc for block 1 - this is already present as highQC from genesis block
	ucs, err := bStore.ProcessQc(block.Qc)
	require.NoError(t, err)
	require.Nil(t, ucs)

	// and the new block 2 - since there is no shards, RootHash stays nil
	rh, err := bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Nil(t, rh)

	// add qc for round 3, commits round 1
	vInfo := &drctypes.RoundInfo{
		RoundNumber:       drctypes.GenesisRootRound + 1,
		ParentRoundNumber: drctypes.GenesisRootRound,
		CurrentRootHash:   rh,
	}
	qc := &drctypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			Hash:                 rBlock.RootHash,
			RootChainRoundNumber: drctypes.GenesisRootRound,
		},
	}
	ucs, err = bStore.ProcessQc(qc)
	require.NoError(t, err)
	require.Empty(t, ucs)

	b, err := bStore.Block(drctypes.GenesisRootRound + 1)
	require.NoError(t, err)
	require.EqualValues(t, drctypes.GenesisRootRound+1, b.BlockData.Round)

	// add block 3
	// qc for round 2, does not commit a round
	block = &drctypes.BlockData{
		Round:   drctypes.GenesisRootRound + 2,
		Payload: &drctypes.Payload{},
		Qc:      qc,
	}
	rh, err = bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Nil(t, rh)
	// prepare block 4
	vInfo = &drctypes.RoundInfo{
		RoundNumber:       drctypes.GenesisRootRound + 2,
		ParentRoundNumber: drctypes.GenesisRootRound + 1,
		CurrentRootHash:   rh,
	}
	qc = &drctypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			Hash:                 rBlock.RootHash,
			RootChainRoundNumber: drctypes.GenesisRootRound + 1,
		},
	}
	// qc for round 4, commits round 2
	block = &drctypes.BlockData{
		Round:   drctypes.GenesisRootRound + 3,
		Payload: &drctypes.Payload{},
		Qc:      qc,
	}
	// Add
	ucs, err = bStore.ProcessQc(qc)
	require.NoError(t, err)
	require.Empty(t, ucs)
	// add block 4
	rh, err = bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Nil(t, rh)
	rBlock = bStore.blockTree.Root()
	// block in round 2 becomes root
	require.Equal(t, uint64(2), rBlock.BlockData.Round)
	hQc := bStore.GetHighQc()
	require.Equal(t, uint64(3), hQc.VoteInfo.RoundNumber)
	// try to read a non-existing block
	b, err = bStore.Block(100)
	require.ErrorContains(t, err, "block for round 100 not found")
	require.Nil(t, b)
}

func TestBlockStoreStoreLastVote(t *testing.T) {
	t.Run("error - store proposal", func(t *testing.T) {
		bStore := initBlockStoreFromGenesis(t, nil)
		proposal := abdrc.ProposalMsg{}
		require.ErrorContains(t, bStore.StoreLastVote(proposal), "unknown vote type")
	})
	t.Run("read blank store", func(t *testing.T) {
		bStore := initBlockStoreFromGenesis(t, nil)
		msg, err := bStore.ReadLastVote()
		require.NoError(t, err)
		require.Nil(t, msg)
	})
	t.Run("ok - store vote", func(t *testing.T) {
		bStore := initBlockStoreFromGenesis(t, nil)
		vote := &abdrc.VoteMsg{Author: "test"}
		require.NoError(t, bStore.StoreLastVote(vote))
		// read back
		msg, err := bStore.ReadLastVote()
		require.NoError(t, err)
		require.IsType(t, &abdrc.VoteMsg{}, msg)
		require.Equal(t, "test", msg.(*abdrc.VoteMsg).Author)
	})
	t.Run("ok - store timeout vote", func(t *testing.T) {
		bStore := initBlockStoreFromGenesis(t, nil)
		vote := &abdrc.TimeoutMsg{Timeout: &drctypes.Timeout{Round: 1}, Author: "test"}
		require.NoError(t, bStore.StoreLastVote(vote))
		// read back
		msg, err := bStore.ReadLastVote()
		require.NoError(t, err)
		require.IsType(t, &abdrc.TimeoutMsg{}, msg)
		require.Equal(t, "test", msg.(*abdrc.TimeoutMsg).Author)
		require.EqualValues(t, 1, msg.(*abdrc.TimeoutMsg).Timeout.Round)
	})
}

func Test_BlockStore_ShardInfo(t *testing.T) {
	shardConf := newShardConf(t)
	bStore := initBlockStoreFromGenesis(t, shardConf)
	si := bStore.ShardInfo(0xFFFFFFFF, types.ShardID{})
	require.Nil(t, si)
}

func Test_BlockStore_StateRoundtrip(t *testing.T) {
	shardConf := newShardConf(t)
	storeA := initBlockStoreFromGenesis(t, shardConf)
	// the genesis block is created with the shard marked as "changed" - clear
	// it as the state message does not carry that information
	clear(storeA.blockTree.root.data.ShardState.Changed)

	// modify Fees to see if they are restored correctly
	si := storeA.ShardInfo(shardConf.PartitionID, shardConf.ShardID)
	si.Fees[si.nodeIDs[0]] = 10

	state, err := storeA.GetState()
	require.NoError(t, err)
	require.NotNil(t, state)

	db, err := memorydb.New()
	require.NoError(t, err)
	// state msg is used to init "shard info registry", the orchestration provides data which is not part of state msg
	storeB, err := NewFromState(crypto.SHA256, state.CommittedHead, db, storeA.orchestration, logger.New(t))
	require.NoError(t, err)
	require.NotNil(t, storeB)

	// two stores should have the same state now
	require.Equal(t, storeA.blockTree.root.data.ShardState.States, storeB.blockTree.root.data.ShardState.States)
	require.Equal(t, storeA.blockTree.root.data.ShardState.Changed, storeB.blockTree.root.data.ShardState.Changed)
	require.Equal(t, storeA.blockTree.root.data.RootHash, storeB.blockTree.root.data.RootHash, "root hash")
	require.Equal(t, storeA.blockTree.root.child, storeB.blockTree.root.child)
	require.Len(t, storeB.blockTree.roundToNode, len(storeA.blockTree.roundToNode))
	for k, v := range storeA.blockTree.roundToNode {
		bNode, ok := storeB.blockTree.roundToNode[k]
		require.True(t, ok)
		require.ElementsMatch(t, v.child, bNode.child)
	}
	require.Equal(t, storeA.blockTree.highQc, storeB.blockTree.highQc)

	siA := storeA.ShardInfo(shardConf.PartitionID, shardConf.ShardID)
	require.NoError(t, err)
	require.NotNil(t, siA)
	require.NoError(t, siA.IsValid())

	siB := storeB.ShardInfo(shardConf.PartitionID, shardConf.ShardID)
	require.NoError(t, err)
	require.NotNil(t, siB)
	require.NoError(t, siB.IsValid())

	require.Equal(t, siA, siB)
}

func Test_BlockStore_persistence(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "blocks.db")
	// init new (empty) DB from genesis
	db, err := boltdb.New(dbPath)
	require.NoError(t, err)
	log := logger.New(t)
	shardConf := newShardConf(t)
	blockA := genesisBlockWithShard(t, shardConf)
	require.NoError(t, db.Write(blockKey(blockA.GetRound()), blockA))

	orchestration := testpartition.NewOrchestration(t, log)
	require.NoError(t, orchestration.AddShardConfig(shardConf))
	storeA, err := New(crypto.SHA256, db, orchestration, logger.New(t))
	require.NoError(t, err)
	require.NotNil(t, storeA)

	// load new DB instance from the non-empty file - must get the same state
	require.NoError(t, db.Close())
	db, err = boltdb.New(dbPath)
	require.NoError(t, err)

	// to make sure we load the state from db send in empty genesis record
	storeB, err := New(crypto.SHA256, db, orchestration, logger.New(t))
	require.NoError(t, err)

	siA := blockA.ShardState.States[types.PartitionShardID{PartitionID: shardConf.PartitionID, ShardID: shardConf.ShardID.Key()}]
	siB := storeB.ShardInfo(shardConf.PartitionID, shardConf.ShardID)
	require.NoError(t, err)
	require.Equal(t, siA, siB)

	blockB, err := storeB.Block(blockA.GetRound())
	require.NoError(t, err)
	require.Equal(t, blockA, blockB)
}

func newShardConf(t *testing.T) *types.PartitionDescriptionRecord {
	node := testutils.NewTestNode(t)
	return &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     1,
		PartitionTypeID: 1,
		ShardID:         types.ShardID{},
		UnitIDLen:       256,
		TypeIDLen:       32,
		T2Timeout:       2500 * time.Millisecond,
		Validators:      []*types.NodeInfo{node.NodeInfo(t)},
	}
}

func genesisBlockWithShard(t *testing.T, shardConf *types.PartitionDescriptionRecord) *ExecutedBlock {
	hashAlgo := crypto.SHA256
	genesisBlock := &drctypes.BlockData{
		Version:   1,
		Author:    "testgenesis",
		Round:     drctypes.GenesisRootRound,
		Epoch:     drctypes.GenesisRootEpoch,
		Timestamp: types.GenesisTime,
	}

	si, err := NewShardInfo(shardConf, hashAlgo)
	require.NoError(t, err)
	psID := types.PartitionShardID{PartitionID: si.PartitionID, ShardID: si.ShardID.Key()}
	commitQc := &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			Version:           1,
			RoundNumber:       genesisBlock.Round,
			Epoch:             genesisBlock.Epoch,
			Timestamp:         genesisBlock.Timestamp,
			ParentRoundNumber: 0, // no parent block
		},
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			NetworkID:            5,
			RootChainRoundNumber: 4,
			Epoch:                0,
			Timestamp:            123,
			Hash:                 []byte{1, 2, 3},
			PreviousHash:         []byte{3, 2, 1},
		},
	}

	eb := &ExecutedBlock{
		BlockData: genesisBlock,
		HashAlgo:  hashAlgo,
		ShardState: ShardStates{
			States:  map[types.PartitionShardID]*ShardInfo{psID: si},
			Changed: ShardSet{psID: struct{}{}},
		},
		Qc:       commitQc,
		CommitQc: commitQc,
	}
	ut, _, err := eb.ShardState.UnicityTree(hashAlgo)
	require.NoError(t, err)
	eb.RootHash = ut.RootHash()
	commitQc.VoteInfo.CurrentRootHash = eb.RootHash

	return eb
}
