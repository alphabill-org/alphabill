package tokens

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/stretchr/testify/require"
)

func TestTokensProcessBlock_empty(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr, err := w.db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(0), blockNr)
	client.SetMaxBlockNumber(1)
	client.SetBlock(&block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      1})
	require.NoError(t, w.Sync(context.Background()))
	blockNr, err = w.db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(1), blockNr)
}
