package storage

import (
	"crypto"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	testpartition "github.com/alphabill-org/alphabill/rootchain/partitions/testutils"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
)

func initBlockStoreFromGenesis(t *testing.T, shardConf *types.PartitionDescriptionRecord) *BlockStore {
	t.Helper()
	dir := t.TempDir()

	db, err := NewBoltStorage(filepath.Join(dir, "blocks.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	log := logger.New(t)
	orchestration, err := partitions.NewOrchestration(5, filepath.Join(dir, "orchestration.db"), log)
	require.NoError(t, err)
	if shardConf != nil {
		require.NoError(t, orchestration.AddShardConfig(shardConf))
		require.NoError(t, db.WriteBlock(genesisBlockWithShard(t, shardConf), true))
	}
	t.Cleanup(func() { _ = orchestration.Close() })

	bStore, err := New(crypto.SHA256, db, orchestration, log)
	require.NoError(t, err)
	return bStore
}

func Test_BlockStore_New(t *testing.T) {
	t.Run("invalid arguments", func(t *testing.T) {
		bs, err := New(crypto.SHA256, nil, nil, nil)
		require.EqualError(t, err, `storage is nil`)
		require.Nil(t, bs)
	})

	t.Run("loading tree failure", func(t *testing.T) {
		expErr := errors.New("failure to load blocks")
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return nil, expErr },
		}
		bs, err := New(crypto.SHA256, db, nil, nil)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, bs)
	})

	t.Run("loading existing state success", func(t *testing.T) {
		orchestration := mockOrchestration{
			shardConfigs: func(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
				return nil, nil
			},
		}

		rootBlock := ExecutedBlock{BlockData: &rctypes.BlockData{Round: 90}, CommitQc: &rctypes.QuorumCert{}}
		db := mockPersistentStore{
			loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&rootBlock}, nil },
		}
		bs, err := New(crypto.SHA256, db, orchestration, nil)
		require.NoError(t, err)
		require.NotNil(t, bs)

		b, err := bs.Block(rootBlock.GetRound())
		require.NoError(t, err)
		require.Equal(t, &rootBlock, b)
	})
}

func TestHandleTcError(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t, nil)
	require.ErrorContains(t, bStore.ProcessTc(nil), "tc is nil")
	tc := &rctypes.TimeoutCert{
		Timeout: &rctypes.Timeout{
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
	qc := &rctypes.QuorumCert{
		VoteInfo: &rctypes.RoundInfo{
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
		verify: func(round uint64, irChReq *rctypes.IRChangeReq) (*types.InputRecord, error) {
			luc, err := bStore.GetCertificate(irChReq.Partition, irChReq.Shard)
			if err != nil {
				return nil, fmt.Errorf("invalid payload: partition %s state is missing: %w", irChReq.Partition, err)
			}
			switch irChReq.CertReason {
			case rctypes.Quorum:
				// NB! there was at least one request, otherwise we would not be here
				return irChReq.Requests[0].InputRecord, nil
			case rctypes.QuorumNotPossible:
			case rctypes.T2Timeout:
				return luc.UC.InputRecord, nil
			}
			return nil, fmt.Errorf("unknown certification reason %v", irChReq.CertReason)
		},
	}
	block := &rctypes.BlockData{
		Round:   rctypes.GenesisRootRound,
		Payload: &rctypes.Payload{},
		Qc:      nil,
	}
	_, err := bStore.Add(block, mockBlockVer)
	require.ErrorContains(t, err, "add block failed: different block for round 1 is already in store")
	block = &rctypes.BlockData{
		Round:   rctypes.GenesisRootRound + 1,
		Payload: &rctypes.Payload{},
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
	vInfo := &rctypes.RoundInfo{
		RoundNumber:       rctypes.GenesisRootRound + 1,
		ParentRoundNumber: rctypes.GenesisRootRound,
		CurrentRootHash:   rh,
	}
	qc := &rctypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			Hash:                 rBlock.RootHash,
			RootChainRoundNumber: rctypes.GenesisRootRound,
		},
	}
	ucs, err = bStore.ProcessQc(qc)
	require.NoError(t, err)
	require.Empty(t, ucs)

	b, err := bStore.Block(rctypes.GenesisRootRound + 1)
	require.NoError(t, err)
	require.EqualValues(t, rctypes.GenesisRootRound+1, b.BlockData.Round)

	// add block 3
	// qc for round 2, does not commit a round
	block = &rctypes.BlockData{
		Round:   rctypes.GenesisRootRound + 2,
		Payload: &rctypes.Payload{},
		Qc:      qc,
	}
	rh, err = bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Nil(t, rh)
	// prepare block 4
	vInfo = &rctypes.RoundInfo{
		RoundNumber:       rctypes.GenesisRootRound + 2,
		ParentRoundNumber: rctypes.GenesisRootRound + 1,
		CurrentRootHash:   rh,
	}
	qc = &rctypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			Hash:                 rBlock.RootHash,
			RootChainRoundNumber: rctypes.GenesisRootRound + 1,
		},
	}
	// qc for round 4, commits round 2
	block = &rctypes.BlockData{
		Round:   rctypes.GenesisRootRound + 3,
		Payload: &rctypes.Payload{},
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

func Test_BlockStoreStore_LastVote(t *testing.T) {
	// just check that calls are routed to persistent storage,
	// there is no other logic in these methods
	orchestration := mockOrchestration{
		shardConfigs: func(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
			return nil, nil
		},
	}

	rootBlock := ExecutedBlock{BlockData: &rctypes.BlockData{Round: 4}, CommitQc: &rctypes.QuorumCert{}}
	var msg any
	db := mockPersistentStore{
		// to avoid init of genesis return committed block
		loadBlocks: func() ([]*ExecutedBlock, error) { return []*ExecutedBlock{&rootBlock}, nil },
		// returning the value passed to write from read verifies that the persistent store methods are called
		readVote: func() (any, error) { return msg, nil },
		writeVote: func(vote any) error {
			msg = vote
			return nil
		},
	}

	bStore, err := New(crypto.SHA256, db, orchestration, nil)
	require.NoError(t, err)
	// the message type is verified by the storage!
	require.NoError(t, bStore.StoreLastVote(920))
	vote, err := bStore.ReadLastVote()
	require.NoError(t, err)
	require.Equal(t, 920, vote)
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

	db, err := NewBoltStorage(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
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
	db, err := NewBoltStorage(dbPath)
	require.NoError(t, err)
	log := logger.New(t)
	shardConf := newShardConf(t)
	blockA := genesisBlockWithShard(t, shardConf)
	require.NoError(t, db.WriteBlock(blockA, false))

	orchestration := testpartition.NewOrchestration(t, log)
	require.NoError(t, orchestration.AddShardConfig(shardConf))
	storeA, err := New(crypto.SHA256, db, orchestration, log)
	require.NoError(t, err)
	require.NotNil(t, storeA)

	// load new DB instance from the non-empty file - must get the same state
	require.NoError(t, db.Close())
	db, err = NewBoltStorage(dbPath)
	require.NoError(t, err)

	// to make sure we load the state from db send in empty genesis record
	storeB, err := New(crypto.SHA256, db, orchestration, log)
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
	genesisBlock := &rctypes.BlockData{
		Version:   1,
		Author:    "testgenesis",
		Round:     rctypes.GenesisRootRound,
		Epoch:     rctypes.GenesisRootEpoch,
		Timestamp: types.GenesisTime,
	}

	si, err := NewShardInfo(shardConf, hashAlgo)
	require.NoError(t, err)
	psID := types.PartitionShardID{PartitionID: si.PartitionID, ShardID: si.ShardID.Key()}
	commitQc := &rctypes.QuorumCert{
		VoteInfo: &rctypes.RoundInfo{
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

type mockPersistentStore struct {
	loadBlocks func() ([]*ExecutedBlock, error)
	writeBlock func(block *ExecutedBlock, root bool) error
	readVote   func() (msg any, err error)
	writeVote  func(vote any) error
	writeTC    func(tc *rctypes.TimeoutCert) error
	readLastTC func() (*rctypes.TimeoutCert, error)
}

func (mps mockPersistentStore) LoadBlocks() ([]*ExecutedBlock, error) {
	return mps.loadBlocks()
}

func (mps mockPersistentStore) WriteBlock(block *ExecutedBlock, root bool) error {
	return mps.writeBlock(block, root)
}

func (mps mockPersistentStore) ReadLastVote() (msg any, err error) {
	return mps.readVote()
}

func (mps mockPersistentStore) WriteVote(vote any) error {
	return mps.writeVote(vote)
}

func (mps mockPersistentStore) WriteTC(tc *rctypes.TimeoutCert) error {
	return mps.writeTC(tc)
}

func (mps mockPersistentStore) ReadLastTC() (*rctypes.TimeoutCert, error) {
	return mps.readLastTC()
}
