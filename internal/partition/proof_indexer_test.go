package partition

import (
	"context"
	"crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/state"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

type mockStateStoreOK struct{}

func TestNewProofIndexer_history_2(t *testing.T) {
	proofDB := memorydb.New()
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

func TestNewProofIndexer_RunLoop(t *testing.T) {
	t.Run("run loop - no history clean-up", func(t *testing.T) {
		proofDB := memorydb.New()
		indexer := NewProofIndexer(crypto.SHA256, proofDB, 0, testlogger.New(t))
		// start indexing loop
		ctx := context.Background()
		done := make(chan struct{})
		go func(dn chan struct{}) {
			indexer.loop(ctx)
			close(done)
		}(done)

		unitID := make([]byte, 32)
		blockRound1 := simulateInput(1, unitID)
		stateMock := mockStateStoreOK{}
		indexer.Handle(blockRound1.Block, stateMock)
		blockRound2 := simulateInput(2, unitID)
		indexer.Handle(blockRound2.Block, stateMock)
		blockRound3 := simulateInput(3, unitID)
		indexer.Handle(blockRound3.Block, stateMock)
		indexer.Close()
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
			t.Error("indexer loop did not exit in time")
		}
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
		proofDB := memorydb.New()
		indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, testlogger.New(t))
		// start indexing loop
		ctx := context.Background()
		done := make(chan struct{})
		go func(dn chan struct{}) {
			indexer.loop(ctx)
			close(done)
		}(done)

		unitID := make([]byte, 32)
		blockRound1 := simulateInput(1, unitID)
		stateMock := mockStateStoreOK{}
		indexer.Handle(blockRound1.Block, stateMock)
		blockRound2 := simulateInput(2, unitID)
		indexer.Handle(blockRound2.Block, stateMock)
		blockRound3 := simulateInput(3, unitID)
		indexer.Handle(blockRound3.Block, stateMock)
		indexer.Close()
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
			t.Error("indexer loop did not exit in time")
		}
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

func (m mockStateStoreOK) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return &state.Unit{}, nil
}

func (m mockStateStoreOK) CreateUnitStateProof(id types.UnitID, logIndex int, uc *types.UnicityCertificate) (*types.UnitStateProof, error) {
	return &types.UnitStateProof{}, nil
}

func simulateInput(round uint64, unitID []byte) *BlockAndState {
	block := &types.Block{
		Header: &types.Header{SystemID: []byte{0, 0, 0, 1}},
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
