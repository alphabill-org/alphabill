package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func Test_Load(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/round-number", r.URL.Path)
		_, err := fmt.Fprint(w, `{"roundNumber": "42"}`)
		require.NoError(t, err)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w, err := New(ttxs.DefaultTokenTxSystemIdentifier, srv.URL, nil, false, nil)
	require.NoError(t, err)

	rn, err := w.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, rn)
}

func Test_ListTokens(t *testing.T) {
	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind backend.Kind, _ wallet.PubKey, _ string, _ int) ([]backend.TokenUnit, string, error) {
			fungible := []backend.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
			}
			nfts := []backend.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: backend.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: backend.NonFungible,
				},
			}
			switch kind {
			case backend.Fungible:
				return fungible, "", nil
			case backend.NonFungible:
				return nfts, "", nil
			case backend.Any:
				return append(fungible, nfts...), "", nil
			}
			return nil, "", fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, be)
	tokens, err := tw.ListTokens(context.Background(), backend.Any, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 4)

	tokens, err = tw.ListTokens(context.Background(), backend.Fungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 2)

	tokens, err = tw.ListTokens(context.Background(), backend.NonFungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 2)
}

func Test_ListTokens_offset(t *testing.T) {
	allTokens := []backend.TokenUnit{
		{
			ID:     test.RandomBytes(32),
			Kind:   backend.Fungible,
			Symbol: "1",
		},
		{
			ID:     test.RandomBytes(32),
			Kind:   backend.Fungible,
			Symbol: "2",
		},
		{
			ID:     test.RandomBytes(32),
			Kind:   backend.Fungible,
			Symbol: "3",
		},
	}

	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind backend.Kind, _ wallet.PubKey, offsetKey string, _ int) ([]backend.TokenUnit, string, error) {
			return getSubarray(allTokens, offsetKey)
		},
	}

	tw := initTestWallet(t, be)
	tokens, err := tw.ListTokens(context.Background(), backend.Any, AllAccounts)
	tokensForAccount := tokens[1]
	require.NoError(t, err)
	require.Len(t, tokensForAccount, len(allTokens))
	dereferencedTokens := make([]backend.TokenUnit, len(tokensForAccount))
	for i := range tokensForAccount {
		dereferencedTokens[i] = *tokensForAccount[i]
	}
	require.Equal(t, allTokens, dereferencedTokens)
}

func Test_ListTokenTypes(t *testing.T) {
	be := &mockTokenBackend{
		getTokenTypes: func(ctx context.Context, kind backend.Kind, _ wallet.PubKey, _ string, _ int) ([]backend.TokenUnitType, string, error) {
			fungible := []backend.TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
			}
			nfts := []backend.TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: backend.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: backend.NonFungible,
				},
			}
			switch kind {
			case backend.Fungible:
				return fungible, "", nil
			case backend.NonFungible:
				return nfts, "", nil
			case backend.Any:
				return append(fungible, nfts...), "", nil
			}
			return nil, "", fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, be)
	types, err := tw.ListTokenTypes(context.Background(), backend.Any)
	require.NoError(t, err)
	require.Len(t, types, 4)

	types, err = tw.ListTokenTypes(context.Background(), backend.Fungible)
	require.NoError(t, err)
	require.Len(t, types, 2)

	types, err = tw.ListTokenTypes(context.Background(), backend.NonFungible)
	require.NoError(t, err)
	require.Len(t, types, 2)
}

func Test_ListTokenTypes_offset(t *testing.T) {
	allTypes := []backend.TokenUnitType{
		{
			ID:     test.RandomBytes(32),
			Symbol: "1",
			Kind:   backend.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "2",
			Kind:   backend.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "3",
			Kind:   backend.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "4",
			Kind:   backend.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "5",
			Kind:   backend.Fungible,
		},
	}
	be := &mockTokenBackend{
		getTokenTypes: func(ctx context.Context, _ backend.Kind, _ wallet.PubKey, offsetKey string, _ int) ([]backend.TokenUnitType, string, error) {
			return getSubarray(allTypes, offsetKey)
		},
	}

	tw := initTestWallet(t, be)
	types, err := tw.ListTokenTypes(context.Background(), backend.Any)
	require.NoError(t, err)
	require.Len(t, types, len(allTypes))
	dereferencedTypes := make([]backend.TokenUnitType, len(types))
	for i := range types {
		dereferencedTypes[i] = *types[i]
	}
	require.Equal(t, allTypes, dereferencedTypes)
}

func TestNewTypes(t *testing.T) {
	t.Parallel()

	recTxs := make(map[string]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		getTypeHierarchy: func(ctx context.Context, id backend.TokenTypeID) ([]backend.TokenUnitType, error) {
			tx, found := recTxs[string(id)]
			if found {
				tokenType := backend.TokenUnitType{ID: tx.UnitId}
				if strings.Contains(tx.TransactionAttributes.TypeUrl, "CreateFungibleTokenTypeAttributes") {
					tokenType.Kind = backend.Fungible
					attrs := &ttxs.CreateFungibleTokenTypeAttributes{}
					require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeId
					tokenType.DecimalPlaces = attrs.DecimalPlaces
				} else {
					tokenType.Kind = backend.NonFungible
					attrs := &ttxs.CreateNonFungibleTokenTypeAttributes{}
					require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeId
				}
				return []backend.TokenUnitType{tokenType}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitId)] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
			return &backend.FeeCreditBill{
				Id:            []byte{1},
				Value:         100000,
				TxHash:        []byte{2},
				FCBlockNumber: 3,
			}, nil
		},
	}
	tw := initTestWallet(t, be)

	t.Run("fungible type", func(t *testing.T) {
		typeId := test.RandomBytes(32)
		a := CreateFungibleTokenTypeAttributes{
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			Icon:                     &Icon{Type: "image/png", Data: []byte{1}},
			DecimalPlaces:            0,
			ParentTypeId:             nil,
			SubTypeCreationPredicate: script.PredicateAlwaysFalse(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
		}
		_, err := tw.NewFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		tx, found := recTxs[string(typeId)]
		require.True(t, found)
		newFungibleTx := &ttxs.CreateFungibleTokenTypeAttributes{}
		require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newFungibleTx))
		require.Equal(t, typeId, tx.UnitId)
		require.Equal(t, a.Symbol, newFungibleTx.Symbol)
		require.Equal(t, a.Name, newFungibleTx.Name)
		require.Equal(t, a.Icon.Type, newFungibleTx.Icon.Type)
		require.Equal(t, a.Icon.Data, newFungibleTx.Icon.Data)
		require.Equal(t, a.DecimalPlaces, newFungibleTx.DecimalPlaces)
		require.EqualValues(t, tx.Timeout(), 11)

		// new subtype
		b := CreateFungibleTokenTypeAttributes{
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			DecimalPlaces:            2,
			ParentTypeId:             typeId,
			SubTypeCreationPredicate: script.PredicateAlwaysFalse(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
		}
		//check decimal places are validated against the parent type
		_, err = tw.NewFungibleType(context.Background(), 1, b, []byte{2}, nil)
		require.ErrorContains(t, err, "parent type requires 0 decimal places, got 2")
	})

	t.Run("non-fungible type", func(t *testing.T) {
		typeId := test.RandomBytes(32)
		a := CreateNonFungibleTokenTypeAttributes{
			Symbol:                   "ABNFT",
			Name:                     "Long name for ABNFT",
			Icon:                     &Icon{Type: "image/svg", Data: []byte{2}},
			ParentTypeId:             nil,
			SubTypeCreationPredicate: script.PredicateAlwaysFalse(),
			TokenCreationPredicate:   script.PredicateAlwaysTrue(),
			InvariantPredicate:       script.PredicateAlwaysTrue(),
		}
		_, err := tw.NewNonFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		tx, found := recTxs[string(typeId)]
		require.True(t, found)
		newNFTTx := &ttxs.CreateNonFungibleTokenTypeAttributes{}
		require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newNFTTx))
		require.Equal(t, typeId, tx.UnitId)
		require.Equal(t, a.Symbol, newNFTTx.Symbol)
		require.Equal(t, a.Icon.Type, newNFTTx.Icon.Type)
		require.Equal(t, a.Icon.Data, newNFTTx.Icon.Data)
	})
}

func TestMintFungibleToken(t *testing.T) {
	recTxs := make([]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			recTxs = append(recTxs, txs.Transactions...)
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
			return &backend.FeeCreditBill{
				Id:            []byte{1},
				Value:         100000,
				TxHash:        []byte{2},
				FCBlockNumber: 3,
			}, nil
		},
	}
	tw := initTestWallet(t, be)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name  string
		accNr uint64
	}{
		{
			name:  "pub key bearer predicate, account 1",
			accNr: uint64(1),
		},
		{
			name:  "pub key bearer predicate, account 2",
			accNr: uint64(2),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeId := test.RandomBytes(32)
			amount := uint64(100)
			key, err := tw.am.GetAccountKey(tt.accNr - 1)
			require.NoError(t, err)
			_, err = tw.NewFungibleToken(context.Background(), tt.accNr, typeId, amount, bearerPredicateFromHash(key.PubKeyHash.Sha256), nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			newToken := &ttxs.MintFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitId)
			require.Len(t, tx.UnitId, 32)
			require.Equal(t, typeId, newToken.Type)
			require.Equal(t, amount, newToken.Value)
			require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), newToken.Bearer)
		})
	}
}

func TestSendFungible(t *testing.T) {
	recTxs := make([]*txsystem.Transaction, 0)
	typeId := test.RandomBytes(32)
	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offsetKey string, limit int) ([]backend.TokenUnit, string, error) {
			return []backend.TokenUnit{
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 3},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 5},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 7},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 18},
			}, "", nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
			return &backend.FeeCreditBill{
				Id:            []byte{1},
				Value:         100000,
				TxHash:        []byte{2},
				FCBlockNumber: 3,
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			recTxs = append(recTxs, txs.Transactions...)
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
	}
	tw := initTestWallet(t, be)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name               string
		targetAmount       uint64
		expectedErrorMsg   string
		verifyTransactions func(t *testing.T)
	}{
		{
			name:         "one bill is transferred",
			targetAmount: 3,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newTransfer := &ttxs.TransferFungibleTokenAttributes{}
				require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
				require.Equal(t, uint64(3), newTransfer.Value)
			},
		},
		{
			name:         "one bill is split",
			targetAmount: 4,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newSplit := &ttxs.SplitFungibleTokenAttributes{}
				require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newSplit))
				require.Equal(t, uint64(4), newSplit.TargetValue)
			},
		},
		{
			name:         "both split and transfer are submitted",
			targetAmount: 26,
			verifyTransactions: func(t *testing.T) {
				var total = uint64(0)
				for _, tx := range recTxs {
					gtx, err := tw.txs.ConvertTx(tx)
					require.NoError(t, err)
					switch ctx := gtx.(type) {
					case ttxs.TransferFungibleToken:
						total += ctx.Value()
					case ttxs.SplitFungibleToken:
						total += ctx.TargetValue()
					default:
						t.Errorf("unexpected tx type: %s", reflect.TypeOf(ctx))
					}
				}
				require.Equal(t, uint64(26), total)
			},
		},
		{
			name:             "insufficient balance",
			targetAmount:     60,
			expectedErrorMsg: "insufficient value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recTxs = make([]*txsystem.Transaction, 0)
			err := tw.SendFungible(context.Background(), 1, typeId, tt.targetAmount, nil, nil)
			if tt.expectedErrorMsg != "" {
				require.ErrorContains(t, err, tt.expectedErrorMsg)
				return
			} else {
				require.NoError(t, err)
			}
			tt.verifyTransactions(t)
		})
	}
}

func TestMintNFT_InvalidInputs(t *testing.T) {
	tokenID := test.RandomBytes(32)
	accNr := uint64(1)
	tests := []struct {
		name       string
		attrs      MintNonFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name: "invalid name",
			attrs: MintNonFungibleTokenAttributes{
				Name: test.RandomString(257),
			},
			wantErrStr: "name exceeds the maximum allowed size of 256 bytes",
		},
		{
			name: "invalid URI",
			attrs: MintNonFungibleTokenAttributes{
				Uri: "invalid_uri",
			},
			wantErrStr: "URI 'invalid_uri' is invalid",
		},
		{
			name: "URI exceeds maximum allowed length",
			attrs: MintNonFungibleTokenAttributes{
				Uri: string(test.RandomBytes(4097)),
			},
			wantErrStr: "URI exceeds the maximum allowed size of 4096 bytes",
		},
		{
			name: "data exceeds maximum allowed length",
			attrs: MintNonFungibleTokenAttributes{
				Data: test.RandomBytes(65537),
			},
			wantErrStr: "data exceeds the maximum allowed size of 65536 bytes",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wallet := &Wallet{}
			got, err := wallet.NewNFT(context.Background(), accNr, tt.attrs, tokenID, nil)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, got)
		})
	}
}

func TestMintNFT(t *testing.T) {
	recTxs := make([]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			recTxs = append(recTxs, txs.Transactions...)
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
			return &backend.FeeCreditBill{
				Id:            []byte{1},
				Value:         100000,
				TxHash:        []byte{2},
				FCBlockNumber: 3,
			}, nil
		},
	}
	tw := initTestWallet(t, be)
	_, _, err := tw.am.AddAccount()
	require.NoError(t, err)

	tests := []struct {
		name          string
		accNr         uint64
		tokenID       backend.TokenID
		validateOwner func(t *testing.T, accNr uint64, tok *ttxs.MintNonFungibleTokenAttributes)
	}{
		{
			name:  "pub key bearer predicate, account 1",
			accNr: uint64(1),
			validateOwner: func(t *testing.T, accNr uint64, tok *ttxs.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:    "pub key bearer predicate, account 1, predefined token ID",
			accNr:   uint64(1),
			tokenID: test.RandomBytes(32),
			validateOwner: func(t *testing.T, accNr uint64, tok *ttxs.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:  "pub key bearer predicate, account 2",
			accNr: uint64(2),
			validateOwner: func(t *testing.T, accNr uint64, tok *ttxs.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := tw.am.GetAccountKey(tt.accNr - 1)
			require.NoError(t, err)
			typeId := []byte{1}
			a := MintNonFungibleTokenAttributes{
				Bearer:              bearerPredicateFromHash(key.PubKeyHash.Sha256),
				NftType:             typeId,
				Uri:                 "https://alphabill.org",
				Data:                nil,
				DataUpdatePredicate: script.PredicateAlwaysTrue(),
			}
			_, err = tw.NewNFT(context.Background(), tt.accNr, a, tt.tokenID, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			newToken := &ttxs.MintNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitId)
			require.Len(t, tx.UnitId, 32)
			if tt.tokenID != nil {
				require.EqualValues(t, tt.tokenID, tx.UnitId)
			}
			require.Equal(t, typeId, newToken.NftType)
			tt.validateOwner(t, tt.accNr, newToken)
		})
	}
}

func TestTransferNFT(t *testing.T) {
	tokens := make(map[string]*backend.TokenUnit)

	recTxs := make(map[string]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		getToken: func(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error) {
			return tokens[string(id)], nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitId)] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
			return &backend.FeeCreditBill{
				Id:            []byte{1},
				Value:         100000,
				TxHash:        []byte{2},
				FCBlockNumber: 3,
			}, nil
		},
	}
	tw := initTestWallet(t, be)

	first := func(s wallet.PubKey, e error) wallet.PubKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		token         *backend.TokenUnit
		key           wallet.PubKey
		validateOwner func(t *testing.T, accNr uint64, key wallet.PubKey, tok *ttxs.TransferNonFungibleTokenAttributes)
	}{
		{
			name:  "to 'always true' predicate",
			token: &backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)},
			key:   nil,
			validateOwner: func(t *testing.T, accNr uint64, key wallet.PubKey, tok *ttxs.TransferNonFungibleTokenAttributes) {
				require.Equal(t, script.PredicateAlwaysTrue(), tok.NewBearer)
			},
		},
		{
			name:  "to public key hash predicate",
			token: &backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)},
			key:   first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accNr uint64, key wallet.PubKey, tok *ttxs.TransferNonFungibleTokenAttributes) {
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(hash.Sum256(key)), tok.NewBearer)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens[string(tt.token.ID)] = tt.token
			err := tw.TransferNFT(context.Background(), 1, tt.token.ID, tt.key, nil)
			require.NoError(t, err)
			tx, found := recTxs[string(tt.token.ID)]
			require.True(t, found)
			require.EqualValues(t, tt.token.ID, tx.UnitId)
			newTransfer := &ttxs.TransferNonFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
			tt.validateOwner(t, 1, tt.key, newTransfer)
		})
	}
}

func TestUpdateNFTData(t *testing.T) {
	tokens := make(map[string]*backend.TokenUnit)

	recTxs := make(map[string]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		getToken: func(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error) {
			return tokens[string(id)], nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitId)] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
			return &backend.FeeCreditBill{
				Id:            []byte{1},
				Value:         100000,
				TxHash:        []byte{2},
				FCBlockNumber: 3,
			}, nil
		},
	}
	tw := initTestWallet(t, be)

	parseNFTDataUpdate := func(t *testing.T, tx *txsystem.Transaction) *ttxs.UpdateNonFungibleTokenAttributes {
		t.Helper()
		newTransfer := &ttxs.UpdateNonFungibleTokenAttributes{}
		require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
		return newTransfer
	}

	tok := &backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), TxHash: test.RandomBytes(32)}
	tokens[string(tok.ID)] = tok

	// test data, backlink and predicate inputs are submitted correctly
	data := test.RandomBytes(64)
	require.NoError(t, tw.UpdateNFTData(context.Background(), 1, tok.ID, data, []*PredicateInput{{Argument: script.PredicateArgumentEmpty()}}))
	tx, found := recTxs[string(tok.ID)]
	require.True(t, found)

	dataUpdate := parseNFTDataUpdate(t, tx)
	require.Equal(t, data, dataUpdate.Data)
	require.EqualValues(t, tok.TxHash, dataUpdate.Backlink)
	require.Equal(t, [][]byte{{script.StartByte}}, dataUpdate.DataUpdateSignatures)

	// test that wallet not only sends the tx, but also reads it correctly
	data2 := test.RandomBytes(64)
	require.NoError(t, tw.UpdateNFTData(context.Background(), 1, tok.ID, data2, []*PredicateInput{{Argument: script.PredicateArgumentEmpty()}, {AccountNumber: 1}}))
	tx, found = recTxs[string(tok.ID)]
	require.True(t, found)
	dataUpdate = parseNFTDataUpdate(t, tx)
	require.NotEqual(t, data, dataUpdate.Data)
	require.Equal(t, data2, dataUpdate.Data)
	require.Len(t, dataUpdate.DataUpdateSignatures, 2)
	require.Equal(t, []byte{script.StartByte}, dataUpdate.DataUpdateSignatures[0])
	require.Len(t, dataUpdate.DataUpdateSignatures[1], 103)
}

func TestFungibleTokenDC(t *testing.T) {
	am := initAccountManager(t)
	pubKey0, err := am.GetPublicKey(0)
	require.NoError(t, err)
	_, pubKey1, err := am.AddAccount()
	require.NoError(t, err)
	typeID1 := test.RandomBytes(32)
	typeID2 := test.RandomBytes(32)
	typeID3 := test.RandomBytes(32)
	var burnedValue = uint64(0)
	accTokens := map[string][]*backend.TokenUnit{
		string(pubKey0): {
			&backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
			&backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
			&backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
			&backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
		},
		string(pubKey1): {
			&backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
		},
	}

	findToken := func(pubKey wallet.PubKey, id backend.TokenID) *backend.TokenUnit {
		tokens, found := accTokens[string(pubKey)]
		require.True(t, found, fmt.Sprintf("key %X not found", pubKey))
		for _, token := range tokens {
			if bytes.Equal(token.ID, id) {
				return token
			}
		}
		t.Fatalf("unit %X not found", id)
		return nil
	}

	recordedTx := make(map[string]*txsystem.Transaction, 0)

	be := &mockTokenBackend{
		getTokens: func(_ context.Context, _ backend.Kind, owner wallet.PubKey, _ string, _ int) ([]backend.TokenUnit, string, error) {
			tokens, found := accTokens[string(owner)]
			if !found {
				return nil, "", fmt.Errorf("no tokens for pubkey '%X'", owner)
			}
			var res []backend.TokenUnit
			for _, tok := range tokens {
				res = append(res, *tok)
			}
			return res, "", nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				unitID := tx.UnitId
				recordedTx[string(unitID)] = tx
				if tx.TransactionAttributes.TypeUrl == "type.googleapis.com/alphabill.tokens.v1.BurnFungibleTokenAttributes" {
					tok := findToken(pubKey, unitID)
					tok.Burned = true
					burnedValue += tok.Amount
				} else if tx.TransactionAttributes.TypeUrl == "type.googleapis.com/alphabill.tokens.v1.JoinFungibleTokenAttributes" {
					tok := findToken(pubKey, unitID)
					attrs := &ttxs.JoinFungibleTokenAttributes{}
					require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
					require.Equal(t, uint64(300), tok.Amount+burnedValue)
				} else {
					return errors.New("unexpected tx")
				}
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			recordedTx, found := recordedTx[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{BlockNumber: 1, Tx: recordedTx, Proof: nil}, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
			return &backend.FeeCreditBill{
				Id:            []byte{1},
				Value:         100000,
				TxHash:        []byte{2},
				FCBlockNumber: 3,
			}, nil
		},
	}
	tw := initTestWallet(t, be)
	tw.am = am

	ctx := context.Background()

	// this should only join tokens with type typeID3
	require.NoError(t, tw.CollectDust(ctx, AllAccounts, nil, nil))
	// tx validation is done in postTransactions()
}

func TestGetTokensForDC(t *testing.T) {
	typeID1 := test.RandomBytes(32)
	typeID2 := test.RandomBytes(32)
	typeID3 := test.RandomBytes(32)

	allTokens := []*backend.TokenUnit{
		{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
		{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
		{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
		{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
		{ID: test.RandomBytes(32), Kind: backend.NonFungible, Symbol: "AB3", TypeID: typeID3},
	}

	be := &mockTokenBackend{
		getTokens: func(_ context.Context, kind backend.Kind, owner wallet.PubKey, _ string, _ int) ([]backend.TokenUnit, string, error) {
			require.Equal(t, backend.Fungible, kind)
			var res []backend.TokenUnit
			for _, tok := range allTokens {
				if tok.Kind != kind {
					continue
				}
				res = append(res, *tok)
			}
			return res, "", nil
		},
	}
	tw := initTestWallet(t, be)
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)

	tests := []struct {
		allowedTypes []backend.TokenTypeID
		expected     map[string][]*backend.TokenUnit
	}{
		{
			allowedTypes: nil,
			expected:     map[string][]*backend.TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: make([]backend.TokenTypeID, 0),
			expected:     map[string][]*backend.TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []backend.TokenTypeID{test.RandomBytes(32)},
			expected:     map[string][]*backend.TokenUnit{},
		},
		{
			allowedTypes: []backend.TokenTypeID{typeID3},
			expected:     map[string][]*backend.TokenUnit{},
		},
		{
			allowedTypes: []backend.TokenTypeID{typeID1},
			expected:     map[string][]*backend.TokenUnit{string(typeID1): allTokens[:2]},
		},
		{
			allowedTypes: []backend.TokenTypeID{typeID2},
			expected:     map[string][]*backend.TokenUnit{string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []backend.TokenTypeID{typeID1, typeID2},
			expected:     map[string][]*backend.TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("%v", test.allowedTypes), func(t *testing.T) {
			tokens, err := tw.getTokensForDC(context.Background(), key, test.allowedTypes)
			require.NoError(t, err)
			require.EqualValues(t, test.expected, tokens)
		})
	}
}

func initTestWallet(t *testing.T, backend TokenBackend) *Wallet {
	t.Helper()
	txs, err := ttxs.New(
		ttxs.WithTrustBase(map[string]crypto.Verifier{"test": nil}),
	)
	require.NoError(t, err)

	return &Wallet{
		txs:     txs,
		am:      initAccountManager(t),
		backend: backend,
	}
}

func initAccountManager(t *testing.T) account.Manager {
	t.Helper()
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))
	return am
}

type mockTokenBackend struct {
	getToken         func(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error)
	getTokens        func(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offsetKey string, limit int) ([]backend.TokenUnit, string, error)
	getTokenTypes    func(ctx context.Context, kind backend.Kind, creator wallet.PubKey, offsetKey string, limit int) ([]backend.TokenUnitType, string, error)
	getRoundNumber   func(ctx context.Context) (uint64, error)
	postTransactions func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error
	getTypeHierarchy func(ctx context.Context, id backend.TokenTypeID) ([]backend.TokenUnitType, error)
	getTxProof       func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	getFeeCreditBill func(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error)
}

func (m *mockTokenBackend) GetToken(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error) {
	if m.getToken != nil {
		return m.getToken(ctx, id)
	}
	return nil, fmt.Errorf("GetToken not implemented")
}

func (m *mockTokenBackend) GetTokens(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offsetKey string, limit int) ([]backend.TokenUnit, string, error) {
	if m.getTokens != nil {
		return m.getTokens(ctx, kind, owner, offsetKey, limit)
	}
	return nil, "", fmt.Errorf("GetTokens not implemented")
}

func (m *mockTokenBackend) GetTokenTypes(ctx context.Context, kind backend.Kind, creator wallet.PubKey, offsetKey string, limit int) ([]backend.TokenUnitType, string, error) {
	if m.getTokenTypes != nil {
		return m.getTokenTypes(ctx, kind, creator, offsetKey, limit)
	}
	return nil, "", fmt.Errorf("GetTokenTypes not implemented")
}

func (m *mockTokenBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	if m.getRoundNumber != nil {
		return m.getRoundNumber(ctx)
	}
	return 0, fmt.Errorf("GetRoundNumber not implemented")
}

func (m *mockTokenBackend) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
	if m.postTransactions != nil {
		return m.postTransactions(ctx, pubKey, txs)
	}
	return fmt.Errorf("PostTransactions not implemented")
}

func (m *mockTokenBackend) GetTypeHierarchy(ctx context.Context, id backend.TokenTypeID) ([]backend.TokenUnitType, error) {
	if m.getTypeHierarchy != nil {
		return m.getTypeHierarchy(ctx, id)
	}
	return nil, fmt.Errorf("GetTypeHierarchy not implemented")
}

func (m *mockTokenBackend) GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	if m.getTxProof != nil {
		return m.getTxProof(ctx, unitID, txHash)
	}
	return nil, fmt.Errorf("GetTxProof not implemented")
}

func (m *mockTokenBackend) GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*backend.FeeCreditBill, error) {
	if m.getFeeCreditBill != nil {
		return m.getFeeCreditBill(ctx, unitID)
	}
	return nil, fmt.Errorf("GetFeeCreditBill not implemented")
}

func getSubarray[T interface{}](array []T, offsetKey string) ([]T, string, error) {
	defaultLimit := 2
	offset := 0
	var err error

	if offsetKey != "" {
		offset, err = strconv.Atoi(offsetKey)
		if err != nil {
			return nil, "", err
		}
	}
	subarray := array[offset:util.Min(offset+defaultLimit, len(array))]
	offset += defaultLimit
	if offset >= len(array) {
		offsetKey = ""
	} else {
		offsetKey = strconv.Itoa(offset)
	}
	return subarray, offsetKey, nil
}
