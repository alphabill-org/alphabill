package tokens

import (
	"bytes"
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/testutils"
	"github.com/alphabill-org/alphabill/testutils/logger"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wallet"
	"github.com/alphabill-org/alphabill/wallet/txsubmitter"
	"github.com/stretchr/testify/require"
)

func TestConfirmUnitsTx_skip(t *testing.T) {
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			return nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend, logger.New(t))
	batch.Add(&txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 1}}}})
	err := batch.SendTx(context.Background(), false)
	require.NoError(t, err)

}

func TestConfirmUnitsTx_ok(t *testing.T) {
	getRoundNumberCalled := false
	getTxProofCalled := false
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (*wallet.RoundNumber, error) {
			getRoundNumberCalled = true
			return &wallet.RoundNumber{RoundNumber: 100, LastIndexedRoundNumber: 100}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			getTxProofCalled = true
			return &wallet.Proof{}, nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend, logger.New(t))
	batch.Add(&txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}}}})
	err := batch.SendTx(context.Background(), true)
	require.NoError(t, err)
	require.True(t, getRoundNumberCalled)
	require.True(t, getTxProofCalled)
}

func TestConfirmUnitsTx_timeout(t *testing.T) {
	getRoundNumberCalled := 0
	getTxProofCalled := 0
	randomID1 := test.RandomBytes(32)
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (*wallet.RoundNumber, error) {
			getRoundNumberCalled++
			if getRoundNumberCalled == 1 {
				return &wallet.RoundNumber{RoundNumber: 100, LastIndexedRoundNumber: 100}, nil
			}
			return &wallet.RoundNumber{RoundNumber: 103, LastIndexedRoundNumber: 103}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			getTxProofCalled++
			if bytes.Equal(unitID, randomID1) {
				return &wallet.Proof{}, nil
			}
			return nil, nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend, logger.New(t))
	sub1 := &txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}}}, UnitID: randomID1}
	batch.Add(sub1)
	sub2 := &txsubmitter.TxSubmission{Transaction: &types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 102}}}}
	batch.Add(sub2)
	err := batch.SendTx(context.Background(), true)
	require.ErrorContains(t, err, "confirmation timeout")
	require.EqualValues(t, 2, getRoundNumberCalled)
	require.EqualValues(t, 2, getTxProofCalled)
	require.True(t, sub1.Confirmed())
	require.False(t, sub2.Confirmed())
}

func TestCachingRoundNumberFetcher(t *testing.T) {
	getRoundNumberCalled := 0
	backend := &mockTokenBackend{
		getRoundNumber: func(ctx context.Context) (*wallet.RoundNumber, error) {
			getRoundNumberCalled++
			return &wallet.RoundNumber{RoundNumber: 100, LastIndexedRoundNumber: 100}, nil
		},
	}
	fetcher := &cachingRoundNumberFetcher{delegate: backend.GetRoundNumber}
	rnr, err := fetcher.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 100, rnr.RoundNumber)
	require.EqualValues(t, 100, rnr.LastIndexedRoundNumber)
	require.EqualValues(t, 1, getRoundNumberCalled)
	rnr, err = fetcher.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 100, rnr.RoundNumber)
	require.EqualValues(t, 100, rnr.LastIndexedRoundNumber)
	require.EqualValues(t, 1, getRoundNumberCalled)
}
