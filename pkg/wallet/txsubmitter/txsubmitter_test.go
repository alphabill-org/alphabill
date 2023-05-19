package txsubmitter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfirmUnitsTx_canceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	batch := &TxSubmissionBatch{}
	err := batch.confirmUnitsTx(ctx)
	require.ErrorContains(t, err, "confirming transactions interrupted")
	require.ErrorIs(t, err, context.Canceled)
}

func TestConfirmUnitsTx_contextError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	batch := &TxSubmissionBatch{}
	err := batch.confirmUnitsTx(ctx)
	require.ErrorContains(t, err, "confirming transactions interrupted")
}
