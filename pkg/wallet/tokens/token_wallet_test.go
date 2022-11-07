package tokens

import (
	"context"
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

func TestNewFungibleType(t *testing.T) {
	tw, abClient := createTestWallet(t)
	typeId := []byte{1}
	a := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                            "AB",
		DecimalPlaces:                     0,
		ParentTypeId:                      nil,
		SubTypeCreationPredicateSignature: nil,
		SubTypeCreationPredicate:          script.PredicateAlwaysFalse(),
		TokenCreationPredicate:            script.PredicateAlwaysTrue(),
		InvariantPredicate:                script.PredicateAlwaysTrue(),
	}
	_, err := tw.NewFungibleType(context.Background(), a, typeId)
	require.NoError(t, err)
	txs := abClient.GetRecordedTransactions()
	require.Len(t, txs, 1)
	tx := txs[0]
	newFungibleTx := &tokens.CreateFungibleTokenTypeAttributes{}
	require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newFungibleTx))
	require.Equal(t, typeId, tx.UnitId)
	require.Equal(t, a.Symbol, newFungibleTx.Symbol)
	require.Equal(t, a.DecimalPlaces, newFungibleTx.DecimalPlaces)
}

func TestNewNonFungibleType(t *testing.T) {
	tw, abClient := createTestWallet(t)
	typeId := []byte{2}
	a := &tokens.CreateNonFungibleTokenTypeAttributes{
		Symbol:                            "ABNFT",
		ParentTypeId:                      nil,
		SubTypeCreationPredicateSignature: nil,
		SubTypeCreationPredicate:          script.PredicateAlwaysFalse(),
		TokenCreationPredicate:            script.PredicateAlwaysTrue(),
		InvariantPredicate:                script.PredicateAlwaysTrue(),
	}
	_, err := tw.NewNonFungibleType(context.Background(), a, typeId)
	require.NoError(t, err)
	txs := abClient.GetRecordedTransactions()
	require.Len(t, txs, 1)
	tx := txs[0]
	newNFTTx := &tokens.CreateNonFungibleTokenTypeAttributes{}
	require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newNFTTx))
	require.Equal(t, typeId, tx.UnitId)
	require.Equal(t, a.Symbol, newNFTTx.Symbol)
}

func TestNewFungibleToken_trueBearer(t *testing.T) {
	tw, abClient := createTestWallet(t)
	typeId := []byte{1}
	amount := uint64(100)
	a := &tokens.MintFungibleTokenAttributes{
		Type:                            typeId,
		Value:                           amount,
		TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
	}
	_, err := tw.NewFungibleToken(context.Background(), 0, a)
	require.NoError(t, err)
	txs := abClient.GetRecordedTransactions()
	require.Len(t, txs, 1)
	tx := txs[0]
	newToken := &tokens.MintFungibleTokenAttributes{}
	require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newToken))
	require.NotEqual(t, []byte{0}, tx.UnitId)
	require.Equal(t, typeId, newToken.Type)
	require.Equal(t, amount, newToken.Value)
	require.Equal(t, script.PredicateAlwaysTrue(), newToken.Bearer)
}

func TestNewFungibleToken_pubKeyBearer(t *testing.T) {
	tw, abClient := createTestWallet(t)
	typeId := []byte{1}
	amount := uint64(100)
	a := &tokens.MintFungibleTokenAttributes{
		Type:                            typeId,
		Value:                           amount,
		TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
	}
	accNr := uint64(1)
	_, err := tw.NewFungibleToken(context.Background(), accNr, a)
	require.NoError(t, err)
	txs := abClient.GetRecordedTransactions()
	require.Len(t, txs, 1)
	tx := txs[0]
	newToken := &tokens.MintFungibleTokenAttributes{}
	require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newToken))
	require.NotEqual(t, []byte{0}, tx.UnitId)
	require.Equal(t, typeId, newToken.Type)
	require.Equal(t, amount, newToken.Value)
	key, err := tw.mw.GetAccountKey(accNr - 1)
	require.NoError(t, err)
	require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), newToken.Bearer)
}

func createTestWallet(t *testing.T) (*TokensWallet, *clientmock.MockAlphabillClient) {
	_ = deleteFile(os.TempDir(), money.WalletFileName)
	_ = deleteFile(os.TempDir(), tokensFileName)
	c := money.WalletConfig{DbPath: os.TempDir()}
	w, err := money.CreateNewWallet("", c)
	require.NoError(t, err)
	tw, err := Load(w, false)
	t.Cleanup(func() {
		deleteWallet(tw)
	})
	require.NoError(t, err)

	mockClient := clientmock.NewMockAlphabillClient(0, map[uint64]*block.Block{})
	w.AlphabillClient = mockClient
	return tw, mockClient
}

func deleteFile(dir string, file string) error {
	return os.Remove(path.Join(dir, file))
}

func deleteWallet(w *TokensWallet) {
	if w != nil {
		w.Shutdown()
		w.mw.DeleteDb()
		w.db.DeleteDb()
	}
}
