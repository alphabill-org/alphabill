package partition

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/state"
)

func TestNewProofIndexer_history_2(t *testing.T) {
	proofDB, err := memorydb.New()
	require.NoError(t, err)
	logger := testlogger.New(t)
	indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, logger)
	require.Equal(t, proofDB, indexer.GetDB())

	// start indexing loop
	ctx := context.Background()
	unitID := make([]byte, 32)
	blockRound1 := simulateInput(1, unitID)
	require.NoError(t, indexer.create(ctx, blockRound1.Block, 1, blockRound1.State))
	// make sure when called with bigger round than history nothing bad will happen
	require.NoError(t, indexer.historyCleanup(ctx, 3))

	blockRound2 := simulateInput(2, unitID)
	require.NoError(t, indexer.create(ctx, blockRound2.Block, 2, blockRound2.State))

	blockRound3 := simulateInput(3, unitID)
	require.NoError(t, indexer.create(ctx, blockRound3.Block, 3, blockRound3.State))

	// run clean-up
	require.NoError(t, indexer.historyCleanup(ctx, 3))
	require.EqualValues(t, 3, indexer.latestIndexedBlockNumber())

	// verify round number 1 is correctly cleaned up
	for _, txr := range blockRound1.Block.Transactions {
		// verify tx index is not deleted
		txo, err := txr.GetTransactionOrderV1()
		require.NoError(t, err)
		txoHash := txo.Hash(crypto.SHA256)
		var index *TxIndex
		f, err := proofDB.Read(txoHash, &index)
		require.NoError(t, err)
		require.True(t, f)

		// verify unit proofs are deleted
		var unitProof *types.UnitDataAndProof
		unitProofKey := bytes.Join([][]byte{unitID, txoHash}, nil)
		f, err = proofDB.Read(unitProofKey, &unitProof)
		require.NoError(t, err)
		require.False(t, f)
	}
}

func TestNewProofIndexer_IndexBlock_EmptyInput(t *testing.T) {
	proofDB, err := memorydb.New()
	require.NoError(t, err)
	logger := testlogger.New(t)
	indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, logger)
	require.Equal(t, proofDB, indexer.GetDB())
	// start indexing loop
	ctx := context.Background()
	blockRound1 := simulateEmptyInput(1)
	require.NoError(t, indexer.IndexBlock(ctx, blockRound1.Block, 1, blockRound1.State))
	blockRound2 := simulateEmptyInput(2)
	require.NoError(t, indexer.IndexBlock(ctx, blockRound2.Block, 2, blockRound2.State))
	blockRound3 := simulateEmptyInput(3)
	// index block 1 again will just return as it is already indexed
	require.NoError(t, indexer.IndexBlock(ctx, blockRound1.Block, 1, blockRound1.State))
	// since history is set to 2 rounds/blocks, then 1 will be now removed
	require.NoError(t, indexer.IndexBlock(ctx, blockRound3.Block, 3, blockRound3.State))
	require.EqualValues(t, 3, indexer.latestIndexedBlockNumber())
	// index db contains only latest round number
	dbIt := proofDB.First()
	require.True(t, dbIt.Valid())
	require.Equal(t, keyLatestRoundNumber, dbIt.Key())
	dbIt.Next()
	require.False(t, dbIt.Valid())
	require.NoError(t, dbIt.Close())
}

func TestNewProofIndexer_IndexBlock(t *testing.T) {
	proofDB, err := memorydb.New()
	require.NoError(t, err)
	logger := testlogger.New(t)
	indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, logger)
	require.Equal(t, proofDB, indexer.GetDB())
	// start indexing loop
	ctx := context.Background()
	unit1ID := test.RandomBytes(32)
	blockRound1 := simulateInput(1, unit1ID)
	require.NoError(t, indexer.IndexBlock(ctx, blockRound1.Block, 1, blockRound1.State))
	unit2ID := test.RandomBytes(32)
	blockRound2 := simulateInput(2, unit2ID)
	require.NoError(t, indexer.IndexBlock(ctx, blockRound2.Block, 2, blockRound2.State))
	unit3ID := test.RandomBytes(32)
	blockRound3 := simulateInput(3, unit3ID)
	// since history is set to 2 rounds/blocks, then 1 will be now removed
	require.NoError(t, indexer.IndexBlock(ctx, blockRound3.Block, 3, blockRound3.State))
	require.EqualValues(t, 3, indexer.latestIndexedBlockNumber())
	// index db contains only latest round number
	dbIt := proofDB.Find(util.Uint64ToBytes(1))
	require.True(t, dbIt.Valid())
	require.NotEqual(t, util.Uint64ToBytes(1), dbIt.Key())
	require.NoError(t, dbIt.Close())
	// check for tx proofs, tx proofs are not cleaned
	txo, err := blockRound1.Block.Transactions[0].GetTransactionOrderV1()
	require.NoError(t, err)
	txHash := txo.Hash(crypto.SHA256)
	idx, err := ReadTransactionIndex(proofDB, txHash)
	require.NoError(t, err)
	require.EqualValues(t, idx.TxOrderIndex, 0)
	require.EqualValues(t, idx.RoundNumber, 1)
	txo, err = blockRound2.Block.Transactions[0].GetTransactionOrderV1()
	require.NoError(t, err)
	txHash = txo.Hash(crypto.SHA256)
	idx, err = ReadTransactionIndex(proofDB, txHash)
	require.NoError(t, err)
	require.EqualValues(t, idx.TxOrderIndex, 0)
	require.EqualValues(t, idx.RoundNumber, 2)
	txo, err = blockRound3.Block.Transactions[0].GetTransactionOrderV1()
	require.NoError(t, err)
	txHash = txo.Hash(crypto.SHA256)
	idx, err = ReadTransactionIndex(proofDB, txHash)
	require.NoError(t, err)
	require.EqualValues(t, idx.TxOrderIndex, 0)
	require.EqualValues(t, idx.RoundNumber, 3)
	// try to read a non-existing index
	idx, err = ReadTransactionIndex(proofDB, make([]byte, 32))
	require.ErrorIs(t, err, ErrIndexNotFound)
	require.Nil(t, idx)
}

func TestNewProofIndexer_SimulateWriteError(t *testing.T) {
	proofDB, err := memorydb.New()
	require.NoError(t, err)
	logger := testlogger.New(t)
	indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, logger)
	require.Equal(t, proofDB, indexer.GetDB())
	// start indexing loop
	ctx := context.Background()
	blockRound1 := simulateEmptyInput(1)
	require.NoError(t, indexer.IndexBlock(ctx, blockRound1.Block, 1, blockRound1.State))
	proofDB.MockWriteError(fmt.Errorf("db write error"))
	blockRound2 := simulateEmptyInput(2)
	require.Error(t, indexer.IndexBlock(ctx, blockRound2.Block, 2, blockRound2.State))
}

func TestNewProofIndexer_RunLoop(t *testing.T) {
	t.Run("run loop - no history clean-up", func(t *testing.T) {
		proofDB, err := memorydb.New()
		require.NoError(t, err)
		logger := testlogger.New(t)
		indexer := NewProofIndexer(crypto.SHA256, proofDB, 0, logger)
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
			tx, err := transaction.GetTransactionOrderV1()
			require.NoError(t, err)
			oderHash := tx.Hash(crypto.SHA256)
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
		logger := testlogger.New(t)
		indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, logger)

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

		// verify history for round 1 is correctly cleaned up
		for _, transaction := range blockRound1.Block.Transactions {
			// verify tx index is not deleted
			tx, err := transaction.GetTransactionOrderV1()
			require.NoError(t, err)
			txoHash := tx.Hash(crypto.SHA256)
			var index *TxIndex
			f, err := proofDB.Read(txoHash, &index)
			require.NoError(t, err)
			require.True(t, f)

			// verify unit proofs are deleted
			var unitProof *types.UnitDataAndProof
			unitProofKey := bytes.Join([][]byte{unitID, txoHash}, nil)
			f, err = proofDB.Read(unitProofKey, &unitProof)
			require.NoError(t, err)
			require.False(t, f)
		}
	})
}

func TestProofIndexer_BoltDBTx(t *testing.T) {
	proofDB, err := boltdb.New(filepath.Join(t.TempDir(), "tempdb.db"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = proofDB.Close()
	})
	logger := testlogger.New(t)
	indexer := NewProofIndexer(crypto.SHA256, proofDB, 2, logger)

	// simulate error when indexing a block
	ctx := context.Background()
	bas := simulateInput(1, []byte{1})
	bas.State = mockStateStoreOK{err: errors.New("some error")}

	err = indexer.create(ctx, bas.Block, 1, bas.State)
	require.ErrorContains(t, err, "some error")

	// verify index db does not contain the stored round number (tx is rolled back)
	dbIt := proofDB.First()
	t.Cleanup(func() {
		_ = dbIt.Close()
	})
	require.False(t, dbIt.Valid())
}

type mockStateStoreOK struct {
	err error
}

func (m mockStateStoreOK) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &state.Unit{}, nil
}

func (m mockStateStoreOK) CreateUnitStateProof(id types.UnitID, logIndex int) (*types.UnitStateProof, error) {
	return &types.UnitStateProof{}, nil
}

func (m mockStateStoreOK) CreateIndex(state.KeyExtractor[string]) (state.Index[string], error) {
	return nil, nil
}

func (m mockStateStoreOK) Serialize(writer io.Writer, committed bool) error {
	return nil
}

func simulateInput(round uint64, unitID []byte) *BlockAndState {
	uc, _ := (&types.UnicityCertificate{Version: 1,
		InputRecord: &types.InputRecord{Version: 1, RoundNumber: round},
	}).MarshalCBOR()
	tx, _ := (&types.TransactionOrder{Version: 1, Payload: types.Payload{SystemID: types.SystemID(1), UnitID: unitID}}).MarshalCBOR()
	block := &types.Block{
		Header: &types.Header{Version: 1, SystemID: 1},
		Transactions: []*types.TransactionRecord{
			{
				TransactionOrder: tx,
				ServerMetadata:   &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}},
			},
		},
		UnicityCertificate: uc,
	}
	return &BlockAndState{
		Block: block,
		State: mockStateStoreOK{},
	}
}

func simulateEmptyInput(round uint64) *BlockAndState {
	uc, _ := (&types.UnicityCertificate{Version: 1,
		InputRecord: &types.InputRecord{Version: 1, RoundNumber: round},
	}).MarshalCBOR()
	block := &types.Block{
		Header:             &types.Header{Version: 1, SystemID: 1},
		Transactions:       []*types.TransactionRecord{},
		UnicityCertificate: uc,
	}
	return &BlockAndState{
		Block: block,
		State: mockStateStoreOK{},
	}
}
