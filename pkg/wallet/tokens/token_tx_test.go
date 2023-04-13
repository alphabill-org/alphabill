package tokens

import (
	"bytes"
	"context"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/stretchr/testify/require"
)

func TestConfirmUnitsTx_skip(t *testing.T) {
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
			return nil
		},
	}
	batch := &txSubmissionBatch{backend: backend}
	batch.add(&txSubmission{tx: &txsystem.Transaction{ClientMetadata: &txsystem.ClientMetadata{Timeout: 1}}})
	err := batch.sendTx(context.Background(), false)
	require.NoError(t, err)

}

func TestConfirmUnitsTx_ok(t *testing.T) {
	getRoundNumberCalled := false
	getTxProofCalled := false
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled = true
			return 100, nil
		},
		getTxProof: func(ctx context.Context, unitID twb.UnitID, txHash twb.TxHash) (*twb.Proof, error) {
			getTxProofCalled = true
			return &twb.Proof{}, nil
		},
	}
	batch := &txSubmissionBatch{backend: backend}
	batch.add(&txSubmission{tx: &txsystem.Transaction{ClientMetadata: &txsystem.ClientMetadata{Timeout: 101}}})
	err := batch.sendTx(context.Background(), true)
	require.NoError(t, err)
	require.True(t, getRoundNumberCalled)
	require.True(t, getTxProofCalled)
}

func TestConfirmUnitsTx_timeout(t *testing.T) {
	getRoundNumberCalled := 0
	getTxProofCalled := 0
	randomID1 := test.RandomBytes(32)
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled++
			if getRoundNumberCalled == 1 {
				return 100, nil
			}
			return 102, nil
		},
		getTxProof: func(ctx context.Context, unitID twb.UnitID, txHash twb.TxHash) (*twb.Proof, error) {
			getTxProofCalled++
			if bytes.Equal(unitID, randomID1) {
				return &twb.Proof{}, nil
			}
			return nil, nil
		},
	}
	batch := &txSubmissionBatch{backend: backend}
	sub1 := &txSubmission{tx: &txsystem.Transaction{ClientMetadata: &txsystem.ClientMetadata{Timeout: 101}}, id: randomID1}
	batch.add(sub1)
	sub2 := &txSubmission{tx: &txsystem.Transaction{ClientMetadata: &txsystem.ClientMetadata{Timeout: 102}}}
	batch.add(sub2)
	err := batch.sendTx(context.Background(), true)
	require.ErrorContains(t, err, "confirmation timeout")
	require.EqualValues(t, 2, getRoundNumberCalled)
	require.EqualValues(t, 2, getTxProofCalled)
	require.True(t, sub1.confirmed)
	require.False(t, sub2.confirmed)
}

func TestConfirmUnitsTx_canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	batch := &txSubmissionBatch{}
	err := batch.confirmUnitsTx(ctx)
	require.ErrorContains(t, err, "confirming transactions interrupted")
	require.ErrorIs(t, err, context.Canceled)
}

func TestConfirmUnitsTx_contextError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	batch := &txSubmissionBatch{}
	err := batch.confirmUnitsTx(ctx)
	require.ErrorContains(t, err, "confirming transactions interrupted")
}
