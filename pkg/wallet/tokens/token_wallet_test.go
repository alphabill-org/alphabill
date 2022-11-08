package tokens

import (
	"context"
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
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

func TestNewFungibleToken(t *testing.T) {
	tw, abClient := createTestWallet(t)
	tests := []struct {
		name          string
		accNr         uint64
		validateOwner func(t *testing.T, accNr uint64, tok *tokens.MintFungibleTokenAttributes)
	}{
		{
			name:  "always true bearer predicate",
			accNr: uint64(0),
			validateOwner: func(t *testing.T, accNr uint64, tok *tokens.MintFungibleTokenAttributes) {
				require.Equal(t, script.PredicateAlwaysTrue(), tok.Bearer)
			},
		},
		{
			name:  "pub key bearer predicate",
			accNr: uint64(1),
			validateOwner: func(t *testing.T, accNr uint64, tok *tokens.MintFungibleTokenAttributes) {
				key, err := tw.mw.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeId := []byte{1}
			amount := uint64(100)
			a := &tokens.MintFungibleTokenAttributes{
				Type:                            typeId,
				Value:                           amount,
				TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
			}
			_, err := tw.NewFungibleToken(context.Background(), tt.accNr, a)
			require.NoError(t, err)
			txs := abClient.GetRecordedTransactions()
			tx := txs[len(txs)-1]
			newToken := &tokens.MintFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitId)
			require.Equal(t, typeId, newToken.Type)
			require.Equal(t, amount, newToken.Value)
			tt.validateOwner(t, tt.accNr, newToken)
		})
	}
}

func TestNewNFT(t *testing.T) {
	tw, abClient := createTestWallet(t)
	tests := []struct {
		name          string
		accNr         uint64
		validateOwner func(t *testing.T, accNr uint64, tok *tokens.MintNonFungibleTokenAttributes)
	}{
		{
			name:  "always true bearer predicate",
			accNr: uint64(0),
			validateOwner: func(t *testing.T, accNr uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				require.Equal(t, script.PredicateAlwaysTrue(), tok.Bearer)
			},
		},
		{
			name:  "pub key bearer predicate",
			accNr: uint64(1),
			validateOwner: func(t *testing.T, accNr uint64, tok *tokens.MintNonFungibleTokenAttributes) {
				key, err := tw.mw.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeId := []byte{1}
			a := &tokens.MintNonFungibleTokenAttributes{
				NftType:                         typeId,
				Uri:                             "",
				Data:                            nil,
				DataUpdatePredicate:             script.PredicateAlwaysTrue(),
				TokenCreationPredicateSignature: script.PredicateArgumentEmpty(),
			}
			_, err := tw.NewNFT(context.Background(), tt.accNr, a, nil)
			require.NoError(t, err)
			txs := abClient.GetRecordedTransactions()
			tx := txs[len(txs)-1]
			newToken := &tokens.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitId)
			require.Equal(t, typeId, newToken.NftType)
			tt.validateOwner(t, tt.accNr, newToken)
		})
	}
}

func TestTransferFungible(t *testing.T) {
	tw, abClient := createTestWallet(t)
	err := tw.db.WithTransaction(func(c TokenTxContext) error {
		require.NoError(t, c.SetToken(1, &token{Id: []byte{11}, Kind: FungibleToken, Symbol: "AB", TypeId: []byte{10}, Amount: 1}))
		require.NoError(t, c.SetToken(1, &token{Id: []byte{12}, Kind: FungibleToken, Symbol: "AB", TypeId: []byte{10}, Amount: 2}))
		return nil
	})
	require.NoError(t, err)
	first := func(s PublicKey, e error) PublicKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		tokenId       TokenId
		amount        uint64
		key           PublicKey
		validateOwner func(t *testing.T, accNr uint64, key PublicKey, tok *tokens.TransferFungibleTokenAttributes)
	}{
		{
			name:    "to 'always true' predicate",
			tokenId: []byte{11},
			amount:  1,
			key:     nil,
			validateOwner: func(t *testing.T, accNr uint64, key PublicKey, tok *tokens.TransferFungibleTokenAttributes) {
				require.Equal(t, script.PredicateAlwaysTrue(), tok.NewBearer)
			},
		},
		{
			name:    "to public key hash predicate",
			tokenId: []byte{12},
			amount:  2,
			key:     first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accNr uint64, key PublicKey, tok *tokens.TransferFungibleTokenAttributes) {
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(hash.Sum256(key)), tok.NewBearer)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = tw.Transfer(context.Background(), 1, tt.tokenId, tt.key)
			require.NoError(t, err)
			txs := abClient.GetRecordedTransactions()
			tx := txs[len(txs)-1]
			newTransfer := &tokens.TransferFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
			require.NotEqual(t, tt.tokenId, tx.UnitId)
			require.Equal(t, tt.amount, newTransfer.Value)
			tt.validateOwner(t, 1, tt.key, newTransfer)
		})
	}
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
