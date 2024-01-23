package partition

import (
	"context"
	"crypto"
	"hash"
	"testing"
	"time"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

func TestNewProofIndexer_history_2(t *testing.T) {
	proofDB, err := memorydb.New()
	require.NoError(t, err)
	indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, testlogger.New(t))
	require.Equal(t, proofDB, indexer.GetDB())
	// start indexing loop
	ctx := context.Background()
	unitID := make([]byte, 32)
	blockRound1 := simulateInput(1, unitID)
	require.NoError(t, indexer.create(ctx, blockRound1))
	blockRound2 := simulateInput(2, unitID)
	require.NoError(t, indexer.create(ctx, blockRound2))
	// add the same block again
	require.ErrorContains(t, indexer.create(ctx, blockRound2), "block 2 already indexed")
	blockRound3 := simulateInput(3, unitID)
	require.NoError(t, indexer.create(ctx, blockRound3))
	// run clean-up
	require.NoError(t, indexer.historyCleanup(ctx, 3))
	require.EqualValues(t, 3, indexer.latestIndexedBlockNumber())
	// verify history for round 1 is cleaned up
	for _, transaction := range blockRound1.Block.Transactions {
		oderHash := transaction.TransactionOrder.Hash(crypto.SHA256)
		index := &struct {
			RoundNumber  uint64
			TxOrderIndex int
		}{}
		f, err := proofDB.Read(oderHash, index)
		require.NoError(t, err)
		require.False(t, f)
	}
}

func TestNewProofIndexer_NothingIsWrittenIfBlockIsEmpty(t *testing.T) {
	proofDB, err := memorydb.New()
	require.NoError(t, err)
	indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, testlogger.New(t))
	require.Equal(t, proofDB, indexer.GetDB())
	// start indexing loop
	ctx := context.Background()
	blockRound1 := simulateEmptyInput(1)
	require.NoError(t, indexer.create(ctx, blockRound1))
	blockRound2 := simulateEmptyInput(2)
	require.NoError(t, indexer.create(ctx, blockRound2))
	// add the same block again
	require.ErrorContains(t, indexer.create(ctx, blockRound2), "block 2 already indexed")
	blockRound3 := simulateEmptyInput(3)
	require.NoError(t, indexer.create(ctx, blockRound3))
	// run clean-up
	require.NoError(t, indexer.historyCleanup(ctx, 3))
	require.EqualValues(t, 3, indexer.latestIndexedBlockNumber())
	// index db contains only latest round number
	dbIt := proofDB.First()
	require.True(t, dbIt.Valid())
	require.Equal(t, keyLatestRoundNumber, dbIt.Key())
	dbIt.Next()
	require.False(t, dbIt.Valid())
	require.NoError(t, dbIt.Close())
}

func TestNewProofIndexer_RunLoop(t *testing.T) {
	t.Run("run loop - no history clean-up", func(t *testing.T) {
		proofDB, err := memorydb.New()
		require.NoError(t, err)
		indexer := NewProofIndexer(crypto.SHA256, proofDB, 0, testlogger.New(t))
		// start indexing loop
		ctx := context.Background()
		nctx, cancel := context.WithCancel(ctx)
		done := make(chan error)
		t.Cleanup(func() {
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(200 * time.Millisecond):
				t.Error("indexer loop did not exit in time")
			}
		})
		// start loop
		go func(dn chan error) {
			done <- indexer.loop(nctx)
		}(done)
		unitID := make([]byte, 32)
		blockRound1 := simulateInput(1, unitID)
		stateMock := mockStateStoreOK{}
		indexer.Handle(nctx, blockRound1.Block, stateMock)
		blockRound2 := simulateInput(2, unitID)
		indexer.Handle(nctx, blockRound2.Block, stateMock)
		blockRound3 := simulateInput(3, unitID)
		indexer.Handle(nctx, blockRound3.Block, stateMock)
		require.Eventually(t, func() bool {
			return indexer.latestIndexedBlockNumber() == 3
		}, test.WaitDuration, test.WaitTick)
		// verify history for round 1 is cleaned up
		for _, transaction := range blockRound1.Block.Transactions {
			oderHash := transaction.TransactionOrder.Hash(crypto.SHA256)
			index := &struct {
				RoundNumber  uint64
				TxOrderIndex int
			}{}
			f, err := proofDB.Read(oderHash, index)
			require.NoError(t, err)
			require.True(t, f)
		}
	})
	t.Run("run loop - keep last 2 rounds", func(t *testing.T) {
		proofDB, err := memorydb.New()
		require.NoError(t, err)
		indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, testlogger.New(t))
		// start indexing loop
		ctx := context.Background()
		nctx, cancel := context.WithCancel(ctx)
		done := make(chan error)
		t.Cleanup(func() {
			cancel()
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(200 * time.Millisecond):
				t.Error("indexer loop did not exit in time")
			}
		})
		go func(dn chan error) {
			done <- indexer.loop(nctx)
		}(done)
		unitID := make([]byte, 32)
		blockRound1 := simulateInput(1, unitID)
		stateMock := mockStateStoreOK{}
		indexer.Handle(nctx, blockRound1.Block, stateMock)
		blockRound2 := simulateInput(2, unitID)
		indexer.Handle(nctx, blockRound2.Block, stateMock)
		blockRound3 := simulateInput(3, unitID)
		indexer.Handle(nctx, blockRound3.Block, stateMock)
		require.Eventually(t, func() bool {
			return indexer.latestIndexedBlockNumber() == 3
		}, test.WaitDuration, test.WaitTick)
		// verify history for round 1 is cleaned up
		for _, transaction := range blockRound1.Block.Transactions {
			oderHash := transaction.TransactionOrder.Hash(crypto.SHA256)
			index := &struct {
				RoundNumber  uint64
				TxOrderIndex int
			}{}
			f, err := proofDB.Read(oderHash, index)
			require.NoError(t, err)
			require.False(t, f)
		}
	})
}

func TestNewProofIndexer_OwnerIndex(t *testing.T) {
	t.Run("last owner of unit is added to index", func(t *testing.T) {
		proofIndexer := NewProofIndexer(crypto.SHA256, nil, 0, testlogger.New(t))
		unitID := types.UnitID{1}
		pubKeyHash1 := []byte{1}
		pubKeyHash2 := []byte{2}
		pubKeyHash3 := []byte{3}
		logs := []*state.Log{
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(pubKeyHash1)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(pubKeyHash2)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(pubKeyHash3)},
		}
		err := proofIndexer.indexOwner(unitID, logs)
		require.NoError(t, err)

		// verify that owner index contains only the last owner
		require.Len(t, proofIndexer.ownerUnits, 1)
		ownerUnitIDs := proofIndexer.ownerUnits[string(pubKeyHash3)]
		require.Len(t, ownerUnitIDs, 1)
		require.Equal(t, unitID, ownerUnitIDs[0])
	})
	t.Run("unit is removed from previous owner index (single unit is deleted)", func(t *testing.T) {
		proofIndexer := NewProofIndexer(crypto.SHA256, nil, 0, testlogger.New(t))
		unitID1 := types.UnitID{1}
		unitID2 := types.UnitID{2}
		ownerID1 := []byte{1}
		ownerID2 := []byte{2}

		// set index owner1 => [unit1, unit2]
		proofIndexer.ownerUnits[string(ownerID1)] = []types.UnitID{unitID1, unitID2}

		// transfer unit2 to owner2
		logs := []*state.Log{
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID1)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID2)},
		}
		err := proofIndexer.indexOwner(unitID2, logs)
		require.NoError(t, err)

		// verify that unit2 is removed from owner 1
		owner1UnitIDs := proofIndexer.ownerUnits[string(ownerID1)]
		require.Len(t, owner1UnitIDs, 1)
		require.Equal(t, unitID1, owner1UnitIDs[0])
	})
	t.Run("unit is removed from previous owner index (entire cache entry is deleted)", func(t *testing.T) {
		proofIndexer := NewProofIndexer(crypto.SHA256, nil, 0, testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID1 := []byte{1}
		ownerID2 := []byte{2}

		// set index owner1 => [unit]
		proofIndexer.ownerUnits[string(ownerID1)] = []types.UnitID{unitID}

		// transfer unit to owner2
		logs := []*state.Log{
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID1)},
			{NewBearer: templates.NewP2pkh256BytesFromKeyHash(ownerID2)},
		}
		err := proofIndexer.indexOwner(unitID, logs)
		require.NoError(t, err)

		// verify that unit1 index is removed
		owner1UnitIDs, ok := proofIndexer.ownerUnits[string(ownerID1)]
		require.False(t, ok)
		require.Nil(t, owner1UnitIDs)
	})
	t.Run("non-p2pkh predicate is not indexed", func(t *testing.T) {
		proofIndexer := NewProofIndexer(crypto.SHA256, nil, 0, testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID := templates.AlwaysTrueBytes()
		logs := []*state.Log{{NewBearer: ownerID}}
		err := proofIndexer.indexOwner(unitID, logs)
		require.NoError(t, err)

		// verify that unit is indexed
		ownerUnitIDs := proofIndexer.ownerUnits[string(ownerID)]
		require.Len(t, ownerUnitIDs, 0)
	})
	t.Run("index can be loaded from state", func(t *testing.T) {
		proofIndexer := NewProofIndexer(crypto.SHA256, nil, 0, testlogger.New(t))
		unitID := types.UnitID{1}
		ownerID := templates.AlwaysTrueBytes()
		s := state.NewEmptyState()
		err := s.Apply(state.AddUnit(unitID, ownerID, &mockUnitData{}))
		require.NoError(t, err)
		summaryValue, summaryHash, err := s.CalculateRoot()
		require.NoError(t, err)
		err = s.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
			RoundNumber:  1,
			Hash:         summaryHash,
			SummaryValue: util.Uint64ToBytes(summaryValue),
		}})
		require.NoError(t, err)

		err = proofIndexer.LoadState(s)
		require.NoError(t, err)

		// verify that unit is indexed
		ownerUnitIDs := proofIndexer.ownerUnits[string(ownerID)]
		require.Len(t, ownerUnitIDs, 1)
		require.Equal(t, unitID, ownerUnitIDs[0])
	})
}

type mockStateStoreOK struct{}

func (m mockStateStoreOK) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return &state.Unit{}, nil
}

func (m mockStateStoreOK) CreateUnitStateProof(id types.UnitID, logIndex int) (*types.UnitStateProof, error) {
	return &types.UnitStateProof{}, nil
}

func simulateInput(round uint64, unitID []byte) *BlockAndState {
	block := &types.Block{
		Header: &types.Header{SystemID: 1},
		Transactions: []*types.TransactionRecord{
			{
				TransactionOrder: &types.TransactionOrder{},
				ServerMetadata:   &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}},
			},
		},
		UnicityCertificate: &types.UnicityCertificate{
			InputRecord: &types.InputRecord{RoundNumber: round},
		},
	}
	return &BlockAndState{
		Block: block,
		State: mockStateStoreOK{},
	}
}

func simulateEmptyInput(round uint64) *BlockAndState {
	block := &types.Block{
		Header:       &types.Header{SystemID: 1},
		Transactions: []*types.TransactionRecord{},
		UnicityCertificate: &types.UnicityCertificate{
			InputRecord: &types.InputRecord{RoundNumber: round},
		},
	}
	return &BlockAndState{
		Block: block,
		State: mockStateStoreOK{},
	}
}

type mockUnitData struct{}

func (m mockUnitData) Write(hash.Hash) error { return nil }

func (m mockUnitData) SummaryValueInput() uint64 {
	return 0
}

func (m mockUnitData) Copy() state.UnitData {
	return mockUnitData{}
}
