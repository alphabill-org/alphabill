package tokens

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
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

func TestTokensProcessBlock_withTx(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr := uint64(1)
	id := []byte{0x01}
	tx := createTx(id, blockNr)
	require.NoError(t, anypb.MarshalFrom(tx.TransactionAttributes, &tokens.CreateFungibleTokenTypeAttributes{Symbol: "AB"}, proto.MarshalOptions{}))
	client.SetMaxBlockNumber(blockNr)
	client.SetBlock(&block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      blockNr,
		Transactions:     []*txsystem.Transaction{tx}})
	types, err := w.db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 0)

	require.NoError(t, w.Sync(context.Background()))

	blockNrFromDB, err := w.db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, blockNr, blockNrFromDB)

	types, err = w.db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 1)

	tokenType, err := w.db.Do().GetTokenType(util.Uint256ToBytes(uint256.NewInt(0).SetBytes(id)))
	require.NoError(t, err)
	require.Equal(t, "AB", tokenType.Symbol)
}
