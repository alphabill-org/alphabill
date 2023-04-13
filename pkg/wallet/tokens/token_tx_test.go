package tokens

import (
	"bytes"
	"context"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/stretchr/testify/require"
)

func TestConfirmUnitsTx_skip(t *testing.T) {
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			return nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend)
	batch.Add(&txsubmitter.TxSubmission{Transaction: &txsystem.Transaction{Timeout: 1}})
	err := batch.SendTx(context.Background(), false)
	require.NoError(t, err)

}

func TestConfirmUnitsTx_ok(t *testing.T) {
	getRoundNumberCalled := false
	getTxProofCalled := false
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled = true
			return 100, nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			getTxProofCalled = true
			return &wallet.Proof{}, nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend)
	batch.Add(&txsubmitter.TxSubmission{Transaction: &txsystem.Transaction{Timeout: 101}})
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
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled++
			if getRoundNumberCalled == 1 {
				return 100, nil
			}
			return 102, nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			getTxProofCalled++
			if bytes.Equal(unitID, randomID1) {
				return &wallet.Proof{}, nil
			}
			return nil, nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend)
	sub1 := &txsubmitter.TxSubmission{Transaction: &txsystem.Transaction{Timeout: 101}, UnitID: randomID1}
	batch.Add(sub1)
	sub2 := &txsubmitter.TxSubmission{Transaction: &txsystem.Transaction{Timeout: 102}}
	batch.Add(sub2)
	err := batch.SendTx(context.Background(), true)
	require.ErrorContains(t, err, "confirmation timeout")
	require.EqualValues(t, 2, getRoundNumberCalled)
	require.EqualValues(t, 2, getTxProofCalled)
	require.True(t, sub1.Confirmed)
	require.False(t, sub2.Confirmed)
}

func TestConfirmUnitsTx_canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	batch := &txsubmitter.TxSubmissionBatch{}
	err := batch.ConfirmUnitsTx(ctx)
	require.ErrorContains(t, err, "confirming transactions interrupted")
	require.ErrorIs(t, err, context.Canceled)
}

func TestConfirmUnitsTx_contextError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	batch := &txsubmitter.TxSubmissionBatch{}
	err := batch.ConfirmUnitsTx(ctx)
	require.ErrorContains(t, err, "confirming transactions interrupted")
}
