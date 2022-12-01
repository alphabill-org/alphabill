package tokens

import (
	"context"
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
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
	b := &block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      blockNr,
		Transactions:     []*txsystem.Transaction{tx1, tx2}}
	certifiedBlock, verifiers := testblock.CertifyBlock(t, b, w.txs)
	client.SetBlock(certifiedBlock)

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
	verifyProof(t, tokenType1.ID, tokenType1.Proof, blockNr, tx1, w.txs, verifiers)

	tokenType2, err := w.db.Do().GetTokenType(util.Uint256ToBytes(uint256.NewInt(0).SetBytes(id2)))
	require.NoError(t, err)
	require.Equal(t, "ABNFT", tokenType2.Symbol)
	verifyProof(t, tokenType2.ID, tokenType2.Proof, blockNr, tx2, w.txs, verifiers)
}

func TestTokensProcessBlock_withTx_mintTokens(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr := uint64(1)
	//fungible
	typeId1 := [32]byte{0x10}
	require.NoError(t, w.db.Do().AddTokenType(&TokenUnitType{
		ID:     typeId1[:],
		Symbol: "AB",
	}))
	id1 := []byte{0x11}
	tx1 := createTx(id1, blockNr)
	key, err := w.getAccountKey(1)
	require.NoError(t, err)
	require.NoError(t, anypb.MarshalFrom(tx1.TransactionAttributes, &tokens.MintFungibleTokenAttributes{
		Type:   typeId1[:],
		Value:  uint64(100),
		Bearer: script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256),
	}, proto.MarshalOptions{}))
	//nft
	typeId2 := [32]byte{0x20}
	require.NoError(t, w.db.Do().AddTokenType(&TokenUnitType{
		ID:     typeId2[:],
		Symbol: "ABNFT",
	}))
	id2 := []byte{0x21}
	tx2 := createTx(id2, blockNr)
	require.NoError(t, anypb.MarshalFrom(tx2.TransactionAttributes, &tokens.MintNonFungibleTokenAttributes{
		NftType: typeId2[:],
		Bearer:  script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256),
	}, proto.MarshalOptions{}))
	//build block
	client.SetMaxBlockNumber(blockNr)
	b := &block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      blockNr,
		Transactions:     []*txsystem.Transaction{tx1, tx2}}
	certifiedBlock, verifiers := testblock.CertifyBlock(t, b, w.txs)
	client.SetBlock(certifiedBlock)

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
	require.Equal(t, TokenTypeID(typeId1[:]), token1.GetTypeId())
	verifyProof(t, token1.ID, token1.Proof, blockNr, tx1, w.txs, verifiers)

	token2, err := w.db.Do().GetToken(1, util.Uint256ToBytes(uint256.NewInt(0).SetBytes(id2)))
	require.NoError(t, err)
	require.Equal(t, TokenTypeID(typeId2[:]), token2.GetTypeId())
	verifyProof(t, token2.ID, token2.Proof, blockNr, tx2, w.txs, verifiers)
}

func TestTokensProcessBlock_withTx_sendTokens(t *testing.T) {
	w, client := createTestWallet(t)
	w.sync = true
	defer w.Shutdown()

	acc2idx, pubKey2, err := w.mw.AddAccount()
	acc2 := acc2idx + 1
	require.NoError(t, err)
	//prepare types
	typeID1 := util.Uint256ToBytes(uint256.NewInt(0).SetBytes([]byte{0x10}))
	w.db.Do().AddTokenType(&TokenUnitType{
		ID:   typeID1,
		Kind: FungibleTokenType,
	})
	typeID2 := util.Uint256ToBytes(uint256.NewInt(0).SetBytes([]byte{0x20}))
	w.db.Do().AddTokenType(&TokenUnitType{
		ID:   typeID2,
		Kind: NonFungibleTokenType,
	})
	// prepare tokens on account 1
	id1 := util.Uint256ToBytes(uint256.NewInt(0).SetBytes([]byte{0x11}))
	id2 := util.Uint256ToBytes(uint256.NewInt(0).SetBytes([]byte{0x12}))
	nft1 := util.Uint256ToBytes(uint256.NewInt(0).SetBytes([]byte{0x21}))

	require.NoError(t, w.db.Do().SetToken(1, &TokenUnit{
		TypeID: typeID1,
		ID:     id1,
		Symbol: "AB",
		Amount: 1,
		Kind:   FungibleToken,
	}))
	require.NoError(t, w.db.Do().SetToken(1, &TokenUnit{
		TypeID: typeID1,
		ID:     id2,
		Symbol: "AB",
		Amount: 5,
		Kind:   FungibleToken,
	}))
	require.NoError(t, w.db.Do().SetToken(1, &TokenUnit{
		TypeID: typeID2,
		ID:     nft1,
		Symbol: "ABNFT",
		Kind:   NonFungibleToken,
	}))

	// check db state
	tokens1, err := w.db.Do().GetTokens(1)
	require.NoError(t, err)
	require.Len(t, tokens1, 3)
	tokens2, err := w.db.Do().GetTokens(acc2)
	require.NoError(t, err)
	require.Len(t, tokens2, 0)

	// prepare block
	blockNr := uint64(1)

	// fungible transfer
	tx1 := createTx(id1, blockNr)
	require.NoError(t, err)
	require.NoError(t, anypb.MarshalFrom(tx1.TransactionAttributes, &tokens.TransferFungibleTokenAttributes{
		NewBearer: bearerPredicateFromPubKey(pubKey2),
		Type:      typeID1,
		Value:     uint64(1),
	}, proto.MarshalOptions{}))

	// fungible split
	tx2 := createTx(id2, blockNr)
	require.NoError(t, err)
	require.NoError(t, anypb.MarshalFrom(tx2.TransactionAttributes, &tokens.SplitFungibleTokenAttributes{
		NewBearer:      bearerPredicateFromPubKey(pubKey2),
		Type:           typeID1,
		TargetValue:    2,
		RemainingValue: 3,
	}, proto.MarshalOptions{}))

	// transfer nft
	nftTx := createTx(nft1, blockNr)
	require.NoError(t, anypb.MarshalFrom(nftTx.TransactionAttributes, &tokens.TransferNonFungibleTokenAttributes{
		NewBearer: bearerPredicateFromPubKey(pubKey2),
		NftType:   typeID2,
	}, proto.MarshalOptions{}))

	//build block
	client.SetMaxBlockNumber(blockNr)
	b := &block.Block{
		SystemIdentifier: tokens.DefaultTokenTxSystemIdentifier,
		BlockNumber:      blockNr,
		Transactions:     []*txsystem.Transaction{tx1, tx2, nftTx}}
	certifiedBlock, verifiers := testblock.CertifyBlock(t, b, w.txs)
	client.SetBlock(certifiedBlock)

	require.NoError(t, w.Sync(context.Background()))

	blockNrFromDB, err := w.db.Do().GetBlockNumber()
	require.NoError(t, err)
	require.Equal(t, blockNr, blockNrFromDB)

	tokens1, err = w.db.Do().GetTokens(1)
	require.NoError(t, err)
	// a portion of token2 remains after split
	require.Len(t, tokens1, 1)
	tokens2, err = w.db.Do().GetTokens(acc2)
	require.NoError(t, err)
	require.Len(t, tokens2, 3)

	token1, err := w.db.Do().GetToken(acc2, id1)
	require.NoError(t, err)
	//require.Equal(t, TokenID(id1), token1.GetTypeId()) // TODO type info is missing
	require.Equal(t, FungibleToken, token1.Kind)
	require.Equal(t, uint64(1), token1.Amount)
	verifyProof(t, token1.ID, token1.Proof, blockNr, tx1, w.txs, verifiers)

	// split token
	token2, err := w.db.Do().GetToken(1, id2)
	require.NoError(t, err)
	require.Equal(t, uint64(3), token2.Amount) // 3-2=1
	verifyProof(t, token2.ID, token2.Proof, blockNr, tx2, w.txs, verifiers)

	gtx, err := w.txs.ConvertTx(tx2)
	require.NoError(t, err)
	newId2 := txutil.SameShardIDBytes(uint256.NewInt(0).SetBytes(id2), gtx.(tokens.SplitFungibleToken).HashForIDCalculation(crypto.SHA256))
	splitToken2, err := w.db.Do().GetToken(acc2, newId2)
	require.NoError(t, err)
	//require.Equal(t, TokenID(id2), token2.GetTypeId()) // TODO type info is missing
	require.Equal(t, FungibleToken, splitToken2.Kind)
	require.Equal(t, uint64(2), splitToken2.Amount)
	verifyProof(t, newId2, splitToken2.Proof, blockNr, tx2, w.txs, verifiers)

	// nft
	token3, err := w.db.Do().GetToken(acc2, nft1)
	require.NoError(t, err)
	//require.Equal(t, TokenID(nft1), token3.GetTypeId()) // TODO type info is missing
	require.Equal(t, NonFungibleToken, token3.Kind)
	verifyProof(t, token3.ID, token3.Proof, blockNr, nftTx, w.txs, verifiers)
}

func TestTokensProcessBlock_syncToUnit(t *testing.T) {
	w, client := createTestWallet(t)
	defer w.Shutdown()
	w.sync = true
	blockNr := uint64(1)
	id, err := RandomID()
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
	id, err := RandomID()
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

func verifyProof(t *testing.T, unitID []byte, proof *Proof, blockNo uint64, tx *txsystem.Transaction, txConverter block.TxConverter, verifiers map[string]abcrypto.Verifier) {
	require.NotNil(t, proof)
	require.Equal(t, blockNo, proof.BlockNumber)
	require.Equal(t, tx, proof.Tx)

	blockProof := proof.Proof
	require.NotNil(t, blockProof)
	gtx, err := txConverter.ConvertTx(tx)
	require.NoError(t, err)

	err = blockProof.Verify(unitID, gtx, verifiers, crypto.SHA256)
	require.NoError(t, err)
}
