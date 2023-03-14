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
	batch := &txSubmissionBatch{
		confirmTx: false,
		backend:   backend,
	}
	batch.add(&txSubmission{tx: &txsystem.Transaction{Timeout: 1}})
	err := batch.sendTx(context.Background())
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
	batch := &txSubmissionBatch{
		confirmTx: true,
		backend:   backend,
	}
	batch.add(&txSubmission{tx: &txsystem.Transaction{Timeout: 101}})
	err := batch.sendTx(context.Background())
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
			defer func() { getRoundNumberCalled++ }()
			if getRoundNumberCalled == 0 {
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
	batch := &txSubmissionBatch{
		confirmTx: true,
		backend:   backend,
	}
	sub1 := &txSubmission{tx: &txsystem.Transaction{Timeout: 101}, id: randomID1}
	batch.add(sub1)
	sub2 := &txSubmission{tx: &txsystem.Transaction{Timeout: 102}}
	batch.add(sub2)
	err := batch.sendTx(context.Background())
	require.ErrorContains(t, err, "confirmation timeout")
	require.EqualValues(t, 2, getRoundNumberCalled)
	require.EqualValues(t, 2, getTxProofCalled)
	require.True(t, sub1.confirmed)
	require.False(t, sub2.confirmed)
}
