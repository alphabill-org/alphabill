package storage

import (
	"crypto"
	gocrypto "crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	"github.com/stretchr/testify/require"
)

type MockAlwaysOkBlockVerifier struct {
	blockStore *BlockStore
}

func NewAlwaysOkBlockVerifier(bStore *BlockStore) *MockAlwaysOkBlockVerifier {
	return &MockAlwaysOkBlockVerifier{
		blockStore: bStore,
	}
}

func (m *MockAlwaysOkBlockVerifier) VerifyIRChangeReq(_ uint64, irChReq *drctypes.IRChangeReq) (*InputData, error) {
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot trust the leader is honest
	// Remember all partitions that have changes in the current proposal and apply changes
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	luc, err := m.blockStore.GetCertificate(irChReq.Partition, irChReq.Shard)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: partition %s state is missing: %w", irChReq.Partition, err)
	}
	switch irChReq.CertReason {
	case drctypes.Quorum:
		// NB! there was at least one request, otherwise we would not be here
		return &InputData{IR: irChReq.Requests[0].InputRecord, ShardConfHash: luc.UC.ShardConfHash}, nil
	case drctypes.QuorumNotPossible:
	case drctypes.T2Timeout:
		return &InputData{Partition: irChReq.Partition, IR: luc.UC.InputRecord, ShardConfHash: luc.UC.ShardConfHash}, nil
	}
	return nil, fmt.Errorf("unknown certification reason %v", irChReq.CertReason)
}

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

	bStore, err := New(gocrypto.SHA256, db, orchestration, log)
	require.NoError(t, err)
	return bStore
}

func TestNewBlockStoreFromGenesis(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t, nil)
	hQc := bStore.GetHighQc()
	require.Equal(t, uint64(1), hQc.VoteInfo.RoundNumber)
	require.Nil(t, bStore.IsChangeInProgress(partitionID1, types.ShardID{}))
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
		CurrentIR: make(InputRecords, 0),
		Changed:   make(map[types.PartitionShardID]struct{}),
		HashAlgo:  gocrypto.SHA256,
		RootHash:  make([]byte, 32),
		Qc:        &drctypes.QuorumCert{},
		CommitQc:  nil,
		ShardInfo: shardStates{},
	}
}

func TestNewBlockStoreFromDB_MultipleRoots(t *testing.T) {
	orchestration := testpartition.NewOrchestration(t, logger.New(t))
	db, err := memorydb.New()
	require.NoError(t, err)
	require.NoError(t, storeGenesisInit(db, 5, gocrypto.SHA256))
	// create second root
	vInfo9 := &drctypes.RoundInfo{RoundNumber: 9, ParentRoundNumber: 8}
	h9, err := vInfo9.Hash(gocrypto.SHA256)
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
	h8, err := vInfo8.Hash(gocrypto.SHA256)
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
	h7, err := vInfo7.Hash(gocrypto.SHA256)
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
	bStore, err := New(gocrypto.SHA256, db, orchestration, logger.New(t))
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
	require.NoError(t, storeGenesisInit(db, 5, gocrypto.SHA256))
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
	bStore, err := New(gocrypto.SHA256, db, orchestration, logger.New(t))
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
	bStore, err := New(gocrypto.SHA256, db, testpartition.NewOrchestration(t, log), log)
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
	mockBlockVer := NewAlwaysOkBlockVerifier(bStore)
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

	// modify Fees to see if they are restored correctly
	si := storeA.ShardInfo(shardConf.PartitionID, shardConf.ShardID)
	si.Fees[si.nodeIDs[0]] = 10

	state, err := storeA.GetState()
	require.NoError(t, err)
	require.NotNil(t, state)

	db, err := memorydb.New()
	require.NoError(t, err)
	// state msg is used to init "shard info registry", the orchestration provides data which is not part of state msg
	storeB, err := NewFromState(gocrypto.SHA256, state, db, storeA.orchestration, storeA.log)
	require.NoError(t, err)
	require.NotNil(t, storeB)

	// two stores should have the same state now
	require.ElementsMatch(t, storeA.blockTree.root.data.CurrentIR, storeB.blockTree.root.data.CurrentIR)
	require.Equal(t, storeA.blockTree.root.data.RootHash, storeB.blockTree.root.data.RootHash)
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
	dbPath := t.TempDir()
	// init new (empty) DB from genesis
	db, err := boltdb.New(filepath.Join(dbPath, "blocks.db"))
	require.NoError(t, err)
	log := logger.New(t)
	shardConf := newShardConf(t)
	blockA := genesisBlockWithShard(t, shardConf)
	require.NoError(t, db.Write(blockKey(blockA.GetRound()), blockA))

	orchestration := testpartition.NewOrchestration(t, log)
	require.NoError(t, orchestration.AddShardConfig(shardConf))
	storeA, err := New(gocrypto.SHA256, db, orchestration, log)
	require.NoError(t, err)
	require.NotNil(t, storeA)

	// load new DB instance from the non-empty file - we
	// must get the state we have in the global "pg" setup.
	require.NoError(t, db.Close())
	db, err = boltdb.New(filepath.Join(dbPath, "blocks.db"))
	require.NoError(t, err)

	// to make sure we load the state from db send in empty genesis record
	storeB, err := New(gocrypto.SHA256, db, orchestration, log)
	require.NoError(t, err)

	siA := blockA.ShardInfo[types.PartitionShardID{PartitionID: shardConf.PartitionID, ShardID:shardConf.ShardID.Key()}]
	siB := storeB.ShardInfo(shardConf.PartitionID, shardConf.ShardID)
	require.NoError(t, err)
	require.Equal(t, siA, siB)

	blockB, err := storeB.Block(blockA.GetRound())
	require.NoError(t, err)
	require.Equal(t, blockA, blockB)

	// require.Equal(t, blockA.InputRecord.Epoch, si.LastCR.Technical.Epoch)
	// require.Equal(t, partGenesis.Certificate.InputRecord.RoundNumber+1, si.LastCR.Technical.Round)
	// require.EqualValues(t, partGenesis.Certificate.InputRecord.Hash, si.RootHash)
	// require.Equal(t, partGenesis.Certificate, &si.LastCR.UC, "genesis[%d]", idx)
	// require.Equal(t, partGenesis.PartitionDescription.PartitionID, si.LastCR.Partition)
}

func newShardConf(t *testing.T) *types.PartitionDescriptionRecord {
	node := testutils.NewTestNode(t)
	return &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     partitionID1,
		PartitionTypeID: 1,
		ShardID:         shardID,
		UnitIDLen:       256,
		TypeIDLen:       32,
		T2Timeout:       2500 * time.Millisecond,
		Validators:      []*types.NodeInfo{node.NodeInfo(t)},
	}
}

func storedBlockWithShard(t *testing.T, shardConf *types.PartitionDescriptionRecord) *memorydb.MemoryDB {
	db, err := memorydb.New()
	require.NoError(t, err)
	genesisBlock := genesisBlockWithShard(t, shardConf)
	require.NoError(t, WriteBlock(db, genesisBlock))
	return db
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

	ir, err := NewShardInputData(si, hashAlgo)
	require.NoError(t, err)

	irs := InputRecords{ir}
	ut, _, err := irs.UnicityTree(hashAlgo)
	require.NoError(t, err)

	psID := types.PartitionShardID{PartitionID: si.PartitionID, ShardID: si.ShardID.Key()}
	commitQc := &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			Version:           1,
			RoundNumber:       genesisBlock.Round,
			Epoch:             genesisBlock.Epoch,
			Timestamp:         genesisBlock.Timestamp,
			ParentRoundNumber: 0, // no parent block
			CurrentRootHash:   ut.RootHash(),
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
	return &ExecutedBlock{
		BlockData: genesisBlock,
		HashAlgo:  hashAlgo,
		CurrentIR: irs,
		Changed:   map[types.PartitionShardID]struct{}{},
		ShardInfo: map[types.PartitionShardID]*ShardInfo{psID: si},
		Qc:        commitQc,
		CommitQc:  commitQc,
		RootHash:  ut.RootHash(),
	}
}
