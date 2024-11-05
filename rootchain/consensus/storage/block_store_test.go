package storage

import (
	gocrypto "crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
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
		return &InputData{IR: irChReq.Requests[0].InputRecord, PDRHash: luc.UC.UnicityTreeCertificate.PDRHash}, nil
	case drctypes.QuorumNotPossible:
	case drctypes.T2Timeout:
		return &InputData{Partition: irChReq.Partition, IR: luc.UC.InputRecord, PDRHash: luc.UC.UnicityTreeCertificate.PDRHash}, nil
	}
	return nil, fmt.Errorf("unknown certification reason %v", irChReq.CertReason)
}

func initBlockStoreFromGenesis(t *testing.T) *BlockStore {
	t.Helper()
	db, err := memorydb.New()
	require.NoError(t, err)
	bStore, err := New(gocrypto.SHA256, pg, db, partitions.NewOrchestration(&genesis.RootGenesis{Partitions: pg}))
	require.NoError(t, err)
	return bStore
}

func TestNewBlockStoreFromGenesis(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t)
	hQc := bStore.GetHighQc()
	require.Equal(t, uint64(1), hQc.VoteInfo.RoundNumber)
	require.Nil(t, bStore.IsChangeInProgress(sysID1, types.ShardID{}))
	b, err := bStore.Block(1)
	require.NoError(t, err)
	require.Len(t, b.RootHash, 32)
	_, err = bStore.Block(2)
	require.ErrorContains(t, err, "block for round 2 not found")
	require.Len(t, bStore.GetCertificates(), 2)
	uc, err := bStore.GetCertificate(sysID1, types.ShardID{})
	require.NoError(t, err)
	require.Equal(t, sysID1, uc.UC.UnicityTreeCertificate.Partition)
	uc, err = bStore.GetCertificate(100, types.ShardID{})
	require.Error(t, err)
	require.Nil(t, uc)
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
		Changed:   make(map[partitionShard]struct{}),
		HashAlgo:  gocrypto.SHA256,
		RootHash:  make([]byte, 32),
		Qc:        &drctypes.QuorumCert{},
		CommitQc:  nil,
	}
}

func TestNewBlockStoreFromDB_MultipleRoots(t *testing.T) {
	orchestration := partitions.NewOrchestration(&genesis.RootGenesis{Partitions: pg})
	db, err := memorydb.New()
	require.NoError(t, err)
	require.NoError(t, storeGenesisInit(gocrypto.SHA256, pg, db))
	// create second root
	vInfo9 := &drctypes.RoundInfo{RoundNumber: 9, ParentRoundNumber: 8}
	b10 := fakeBlock(10, &drctypes.QuorumCert{
		VoteInfo: vInfo9,
		LedgerCommitInfo: &types.UnicitySeal{Version: 1,
			PreviousHash: vInfo9.Hash(gocrypto.SHA256),
			Hash:         test.RandomBytes(32),
		},
	})
	require.NoError(t, db.Write(blockKey(b10.GetRound()), b10))
	vInfo8 := &drctypes.RoundInfo{RoundNumber: 8, ParentRoundNumber: 7}
	b9 := fakeBlock(9, &drctypes.QuorumCert{
		VoteInfo: vInfo8,
		LedgerCommitInfo: &types.UnicitySeal{Version: 1,
			PreviousHash:         vInfo8.Hash(gocrypto.SHA256),
			RootChainRoundNumber: 8,
			Hash:                 test.RandomBytes(32),
		},
	})
	b9.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}
	require.NoError(t, db.Write(blockKey(b9.GetRound()), b9))
	vInfo7 := &drctypes.RoundInfo{RoundNumber: 7, ParentRoundNumber: 6}
	b8 := fakeBlock(8, &drctypes.QuorumCert{
		VoteInfo: vInfo7,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			PreviousHash:         vInfo7.Hash(gocrypto.SHA256),
			RootChainRoundNumber: 7,
			Hash:                 test.RandomBytes(32),
		},
	})
	b8.Qc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 8}}
	b8.CommitQc = &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 9}}
	require.NoError(t, db.Write(blockKey(b8.GetRound()), b8))
	// load from DB
	bStore, err := New(gocrypto.SHA256, pg, db, orchestration)
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
	orchestration := partitions.NewOrchestration(&genesis.RootGenesis{Partitions: pg})
	db, err := memorydb.New()
	require.NoError(t, err)
	require.NoError(t, storeGenesisInit(gocrypto.SHA256, pg, db))
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
	bStore, err := New(gocrypto.SHA256, pg, db, orchestration)
	require.ErrorContains(t, err, `initializing block tree: error cannot add block for round 8, parent block 7 not found`)
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
	bStore, err := New(gocrypto.SHA256, pg, db, partitions.NewOrchestration(&genesis.RootGenesis{Partitions: pg}))
	require.ErrorContains(t, err, `reading shard states from storage: shard info {00000001 - } not found`)
	require.Nil(t, bStore)
}

func TestHandleTcError(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t)
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
	bStore := initBlockStoreFromGenesis(t)
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
	bStore := initBlockStoreFromGenesis(t)
	rBlock := bStore.blockTree.Root()
	mockBlockVer := NewAlwaysOkBlockVerifier(bStore)
	block := &drctypes.BlockData{
		Round:   genesis.RootRound,
		Payload: &drctypes.Payload{},
		Qc:      nil,
	}
	_, err := bStore.Add(block, mockBlockVer)
	require.ErrorContains(t, err, "add block failed: different block for round 1 is already in store")
	block = &drctypes.BlockData{
		Round:   genesis.RootRound + 1,
		Payload: &drctypes.Payload{},
		Qc:      bStore.GetHighQc(), // the qc is genesis qc
	}
	// Proposal always comes with Qc and Block, process Qc first and then new block
	// add qc for block 1
	ucs, err := bStore.ProcessQc(block.Qc)
	require.NoError(t, err)
	require.Nil(t, ucs)
	// and the new block 2
	rh, err := bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Len(t, rh, 32)
	// add qc for round 3, commits round 1
	vInfo := &drctypes.RoundInfo{
		RoundNumber:       genesis.RootRound + 1,
		ParentRoundNumber: genesis.RootRound,
		CurrentRootHash:   rh,
	}
	qc := &drctypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{Version: 1,
			Hash: rBlock.RootHash,
		},
	}
	ucs, err = bStore.ProcessQc(qc)
	require.NoError(t, err)
	require.Empty(t, ucs)
	b, err := bStore.Block(genesis.RootRound + 1)
	require.NoError(t, err)
	require.EqualValues(t, genesis.RootRound+1, b.BlockData.Round)
	// add block 3
	// qc for round 2, does not commit a round
	block = &drctypes.BlockData{
		Round:   genesis.RootRound + 2,
		Payload: &drctypes.Payload{},
		Qc:      qc,
	}
	rh, err = bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Len(t, rh, 32)
	// prepare block 4
	vInfo = &drctypes.RoundInfo{
		RoundNumber:       genesis.RootRound + 2,
		ParentRoundNumber: genesis.RootRound + 1,
		CurrentRootHash:   rh,
	}
	qc = &drctypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{Version: 1,
			Hash: rBlock.RootHash,
		},
	}
	// qc for round 2, does not commit a round
	block = &drctypes.BlockData{
		Round:   genesis.RootRound + 3,
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
	require.Len(t, rh, 32)
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
		bStore := initBlockStoreFromGenesis(t)
		proposal := abdrc.ProposalMsg{}
		require.ErrorContains(t, bStore.StoreLastVote(proposal), "unknown vote type")
	})
	t.Run("read blank store", func(t *testing.T) {
		bStore := initBlockStoreFromGenesis(t)
		msg, err := bStore.ReadLastVote()
		require.NoError(t, err)
		require.Nil(t, msg)
	})
	t.Run("ok - store vote", func(t *testing.T) {
		bStore := initBlockStoreFromGenesis(t)
		vote := &abdrc.VoteMsg{Author: "test"}
		require.NoError(t, bStore.StoreLastVote(vote))
		// read back
		msg, err := bStore.ReadLastVote()
		require.NoError(t, err)
		require.IsType(t, &abdrc.VoteMsg{}, msg)
		require.Equal(t, "test", msg.(*abdrc.VoteMsg).Author)
	})
	t.Run("ok - store timeout vote", func(t *testing.T) {
		bStore := initBlockStoreFromGenesis(t)
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
	bStore := initBlockStoreFromGenesis(t)
	// the initBlockStoreFromGenesis fills the store from global var "pg"
	for idx, partGenesis := range pg {
		si, err := bStore.ShardInfo(partGenesis.PartitionDescription.PartitionIdentifier, types.ShardID{})
		require.NoError(t, err)
		require.NotNil(t, si)
		require.Equal(t, partGenesis.Certificate.InputRecord.Epoch, si.Epoch)
		require.Equal(t, partGenesis.Certificate.InputRecord.RoundNumber, si.Round)
		require.Equal(t, partGenesis.Certificate.InputRecord.Hash, si.RootHash)
		require.Equal(t, partGenesis.Certificate, &si.LastCR.UC, "genesis[%d]", idx)
		require.Equal(t, partGenesis.PartitionDescription.PartitionIdentifier, si.LastCR.Partition)
	}

	// and an partition which shouldn't exist
	si, err := bStore.ShardInfo(0xFFFFFFFF, types.ShardID{})
	require.EqualError(t, err, `no shard info found for {FFFFFFFF : }`)
	require.Nil(t, si)
}

func Test_BlockStore_StateRoundtrip(t *testing.T) {
	storeA := initBlockStoreFromGenesis(t)
	state := storeA.GetState()
	require.NotNil(t, state)

	db, err := memorydb.New()
	require.NoError(t, err)
	// state msg is used to init "shard info registry", the orchestration provides data which is not part of state msg
	storeB, err := NewFromState(gocrypto.SHA256, state, db, partitions.NewOrchestration(&genesis.RootGenesis{Partitions: pg}))
	require.NoError(t, err)
	require.NotNil(t, storeB)

	// two stores should have the same state now
	require.Len(t, storeA.shardInfo, len(storeB.shardInfo))
	for _, partGenesis := range pg {
		siA, err := storeA.ShardInfo(partGenesis.PartitionDescription.PartitionIdentifier, types.ShardID{})
		require.NoError(t, err)
		require.NotNil(t, siA)
		siB, err := storeB.ShardInfo(partGenesis.PartitionDescription.PartitionIdentifier, types.ShardID{})
		require.NoError(t, err)
		require.NotNil(t, siB)

		require.Equal(t, siA, siB)
	}
}

func Test_BlockStore_persistance(t *testing.T) {
	dbPath := t.TempDir()
	// init new (empty) DB from genesis
	db, err := boltdb.New(filepath.Join(dbPath, "blocks.db"))
	require.NoError(t, err)
	bStore, err := New(gocrypto.SHA256, pg, db, partitions.NewOrchestration(&genesis.RootGenesis{Partitions: pg}))
	require.NoError(t, err)
	require.NotNil(t, bStore)

	// change some shard info and persist it
	certs := []*certification.CertificationResponse{
		{
			Partition: pg[0].PartitionDescription.PartitionIdentifier,
			Shard:     types.ShardID{},
			Technical: certification.TechnicalRecord{
				Round:    8876,
				Epoch:    pg[0].Certificate.InputRecord.Epoch,
				Leader:   "1234567890",
				StatHash: test.RandomBytes(32),
				FeeHash:  test.RandomBytes(32),
			},
			UC: types.UnicityCertificate{
				Version: 1,
				InputRecord: &types.InputRecord{
					Version:     1,
					RoundNumber: 8875,
					Epoch:       pg[0].Certificate.InputRecord.Epoch,
					Hash:        test.RandomBytes(32),
				},
			},
		},
	}
	// updateCertificateCache updates only LastCR unless it's epoch change too
	// so set some SI properties manually before triggering storing ths state
	si, err := bStore.ShardInfo(pg[0].PartitionDescription.PartitionIdentifier, types.ShardID{})
	require.NoError(t, err)
	si.Round = certs[0].Technical.Round
	si.RootHash = certs[0].UC.InputRecord.Hash
	require.NoError(t, bStore.updateCertificateCache(certs))

	// load new DB instance from the non-empty file - we
	// must get the state we had
	require.NoError(t, db.Close())
	db, err = boltdb.New(filepath.Join(dbPath, "blocks.db"))
	require.NoError(t, err)
	bStore, err = New(gocrypto.SHA256, pg, db, partitions.NewOrchestration(&genesis.RootGenesis{Partitions: pg}))
	require.NoError(t, err)

	cr := certs[0]
	si, err = bStore.ShardInfo(pg[0].PartitionDescription.PartitionIdentifier, types.ShardID{})
	require.NoError(t, err)
	require.NotNil(t, si)
	require.Equal(t, cr.Technical.Epoch, si.Epoch)
	require.Equal(t, cr.Technical.Round, si.Round)
	require.Equal(t, cr.UC.InputRecord.Hash, si.RootHash)
	require.Equal(t, cr.UC, si.LastCR.UC)
	require.Equal(t, cr.Technical, si.LastCR.Technical)
}
