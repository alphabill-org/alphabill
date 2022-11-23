package tokens

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/script"
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

func TestTokensProcessBlock_withTx_tokenTypes(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr := uint64(1)
	//fungible
	id1 := []byte{0x01}
	tx1 := createTx(id1, blockNr)
	require.NoError(t, anypb.MarshalFrom(tx1.TransactionAttributes, &tokens.CreateFungibleTokenTypeAttributes{Symbol: "AB"}, proto.MarshalOptions{}))
	//nft
	id2 := []byte{0x02}
	tx2 := createTx(id2, blockNr)
	require.NoError(t, anypb.MarshalFrom(tx2.TransactionAttributes, &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "ABNFT"}, proto.MarshalOptions{}))
	//build block
	client.SetMaxBlockNumber(blockNr)
	client.SetBlock(&block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      blockNr,
		Transactions:     []*txsystem.Transaction{tx1, tx2}})
	types, err := w.db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 0)

	require.NoError(t, w.Sync(context.Background()))

	blockNrFromDB, err := w.db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, blockNr, blockNrFromDB)

	types, err = w.db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 2)

	tokenType1, err := w.db.Do().GetTokenType(util.Uint256ToBytes(uint256.NewInt(0).SetBytes(id1)))
	require.NoError(t, err)
	require.Equal(t, "AB", tokenType1.Symbol)

	tokenType2, err := w.db.Do().GetTokenType(util.Uint256ToBytes(uint256.NewInt(0).SetBytes(id2)))
	require.NoError(t, err)
	require.Equal(t, "ABNFT", tokenType2.Symbol)
}

func TestTokensProcessBlock_withTx_mintTokens(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr := uint64(1)
	//fungible
	typeId1 := []byte{0x10}
	id1 := []byte{0x11}
	tx1 := createTx(id1, blockNr)
	key, err := w.getAccountKey(1)
	require.NoError(t, err)
	require.NoError(t, anypb.MarshalFrom(tx1.TransactionAttributes, &tokens.MintFungibleTokenAttributes{
		Type:   typeId1,
		Value:  uint64(100),
		Bearer: script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256),
	}, proto.MarshalOptions{}))
	//nft
	typeId2 := []byte{0x20}
	id2 := []byte{0x21}
	tx2 := createTx(id2, blockNr)
	require.NoError(t, anypb.MarshalFrom(tx2.TransactionAttributes, &tokens.MintNonFungibleTokenAttributes{
		NftType: typeId2,
		Bearer:  script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256),
	}, proto.MarshalOptions{}))
	//build block
	client.SetMaxBlockNumber(blockNr)
	client.SetBlock(&block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      blockNr,
		Transactions:     []*txsystem.Transaction{tx1, tx2}})
	tokens, err := w.db.Do().GetTokens(1)
	require.NoError(t, err)
	require.Len(t, tokens, 0)

	require.NoError(t, w.Sync(context.Background()))

	blockNrFromDB, err := w.db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, blockNr, blockNrFromDB)

	tokens, err = w.db.Do().GetTokens(1)
	require.NoError(t, err)
	require.Len(t, tokens, 2)

	token1, err := w.db.Do().GetToken(1, util.Uint256ToBytes(uint256.NewInt(0).SetBytes(id1)))
	require.NoError(t, err)
	require.Equal(t, TokenTypeID(typeId1), token1.GetTypeId())

	token2, err := w.db.Do().GetToken(1, util.Uint256ToBytes(uint256.NewInt(0).SetBytes(id2)))
	require.NoError(t, err)
	require.Equal(t, TokenTypeID(typeId2), token2.GetTypeId())
}

func TestTokensProcessBlock_syncToUnit(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr := uint64(1)
	id, err := RandomId()
	require.NoError(t, err)
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

	require.NoError(t, w.syncToUnit(context.Background(), id, blockNr))

	blockNrFromDB, err := w.db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, blockNr, blockNrFromDB)

	types, err = w.db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 1)

	tokenType, err := w.db.Do().GetTokenType(TokenTypeID(id))
	require.NoError(t, err)
	require.Equal(t, "AB", tokenType.Symbol)
}

func TestTokensProcessBlock_syncToUnit_timeout(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr := uint64(1)
	id, err := RandomId()
	require.NoError(t, err)
	client.SetMaxBlockNumber(blockNr)
	client.SetBlock(&block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      blockNr,
		Transactions:     []*txsystem.Transaction{}})
	types, err := w.db.Do().GetTokenTypes()
	require.NoError(t, err)
	require.Len(t, types, 0)

	require.ErrorContains(t, w.syncToUnit(context.Background(), id, blockNr), "did not confirm all transactions, timed out: 1")
}
