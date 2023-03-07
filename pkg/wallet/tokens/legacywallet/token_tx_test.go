package legacywallet

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

func TestSendTx_WithCorrectTimeout(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()

	client.SetMaxBlockNumber(1)
	client.SetMaxRoundNumber(100)
	client.SetTxListener(func(tx *txsystem.Transaction) {
		_, rn, _ := client.GetMaxBlockNumber()
		require.Equal(t, rn+txTimeoutBlockCount, tx.Timeout)
	})
	_, err := w.sendTx(randomID(t), newFungibleTransferTxAttrs(&TokenUnit{}, nil), nil, nil)
	require.NoError(t, err)
}

func randomID(t *testing.T) TokenID {
	id, err := RandomID()
	require.NoError(t, err)
	return id
}
