package tokens

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
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

	w, err := New(ttxs.DefaultTokenTxSystemIdentifier, srv.URL, nil)
	require.NoError(t, err)

	rn, err := w.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, rn)
}

func Test_ListTokens(t *testing.T) {
	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind twb.Kind, _ twb.PubKey, _ string, _ int) ([]twb.TokenUnit, string, error) {
			fungible := []twb.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
			}
			nfts := []twb.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
			}
			switch kind {
			case twb.Fungible:
				return fungible, "", nil
			case twb.NonFungible:
				return nfts, "", nil
			case twb.Any:
				return append(fungible, nfts...), "", nil
			}
			return nil, "", fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, be)
	tokens, err := tw.ListTokens(context.Background(), twb.Any, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 4)

	tokens, err = tw.ListTokens(context.Background(), twb.Fungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 2)

	tokens, err = tw.ListTokens(context.Background(), twb.NonFungible, AllAccounts)
	require.NoError(t, err)
	require.Len(t, tokens[1], 2)
}

func Test_ListTokens_offset(t *testing.T) {
	allTokens := []twb.TokenUnit{
		{
			ID:     test.RandomBytes(32),
			Kind:   twb.Fungible,
			Symbol: "1",
		},
		{
			ID:     test.RandomBytes(32),
			Kind:   twb.Fungible,
			Symbol: "2",
		},
		{
			ID:     test.RandomBytes(32),
			Kind:   twb.Fungible,
			Symbol: "3",
		},
	}

	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind twb.Kind, _ twb.PubKey, offsetKey string, _ int) ([]twb.TokenUnit, string, error) {
			return getSubarray(allTokens, offsetKey)
		},
	}

	tw := initTestWallet(t, be)
	tokens, err := tw.ListTokens(context.Background(), twb.Any, AllAccounts)
	tokensForAccount := tokens[1]
	require.NoError(t, err)
	require.Len(t, tokensForAccount, len(allTokens))
	dereferencedTokens := make([]twb.TokenUnit, len(tokensForAccount))
	for i := range tokensForAccount {
		dereferencedTokens[i] = *tokensForAccount[i]
	}
	require.Equal(t, allTokens, dereferencedTokens)
}

func Test_ListTokenTypes(t *testing.T) {
	be := &mockTokenBackend{
		getTokenTypes: func(ctx context.Context, kind twb.Kind, _ twb.PubKey, _ string, _ int) ([]twb.TokenUnitType, string, error) {
			fungible := []twb.TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.Fungible,
				},
			}
			nfts := []twb.TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: twb.NonFungible,
				},
			}
			switch kind {
			case twb.Fungible:
				return fungible, "", nil
			case twb.NonFungible:
				return nfts, "", nil
			case twb.Any:
				return append(fungible, nfts...), "", nil
			}
			return nil, "", fmt.Errorf("invalid kind")
		},
	}

	tw := initTestWallet(t, be)
	types, err := tw.ListTokenTypes(context.Background(), twb.Any)
	require.NoError(t, err)
	require.Len(t, types, 4)

	types, err = tw.ListTokenTypes(context.Background(), twb.Fungible)
	require.NoError(t, err)
	require.Len(t, types, 2)

	types, err = tw.ListTokenTypes(context.Background(), twb.NonFungible)
	require.NoError(t, err)
	require.Len(t, types, 2)
}

func Test_ListTokenTypes_offset(t *testing.T) {
	allTypes := []twb.TokenUnitType{
		{
			ID:     test.RandomBytes(32),
			Symbol: "1",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "2",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "3",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "4",
			Kind:   twb.Fungible,
		},
		{
			ID:     test.RandomBytes(32),
			Symbol: "5",
			Kind:   twb.Fungible,
		},
	}
	be := &mockTokenBackend{
		getTokenTypes: func(ctx context.Context, _ twb.Kind, _ twb.PubKey, offsetKey string, _ int) ([]twb.TokenUnitType, string, error) {
			return getSubarray(allTypes, offsetKey)
		},
	}

	tw := initTestWallet(t, be)
	types, err := tw.ListTokenTypes(context.Background(), twb.Any)
	require.NoError(t, err)
	require.Len(t, types, len(allTypes))
	dereferencedTypes := make([]twb.TokenUnitType, len(types))
	for i := range types {
		dereferencedTypes[i] = *types[i]
	}
	require.Equal(t, allTypes, dereferencedTypes)
}

func TestNewTypes(t *testing.T) {
	t.Parallel()

	recTxs := make(map[string]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		getTypeHierarchy: func(ctx context.Context, id twb.TokenTypeID) ([]twb.TokenUnitType, error) {
			tx, found := recTxs[string(id)]
			if found {
				tokenType := twb.TokenUnitType{ID: tx.UnitId}
				if strings.Contains(tx.TransactionAttributes.TypeUrl, "CreateFungibleTokenTypeAttributes") {
					tokenType.Kind = twb.Fungible
					attrs := &ttxs.CreateFungibleTokenTypeAttributes{}
					require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeId
					tokenType.DecimalPlaces = attrs.DecimalPlaces
				} else {
					tokenType.Kind = twb.NonFungible
					attrs := &ttxs.CreateNonFungibleTokenTypeAttributes{}
					require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeId
				}
				return []twb.TokenUnitType{tokenType}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitId)] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
	}
	tw := initTestWallet(t, be)

	t.Run("fungible type", func(t *testing.T) {
		typeId := test.RandomBytes(32)
		a := &ttxs.CreateFungibleTokenTypeAttributes{
			Symbol:                             "AB",
			DecimalPlaces:                      0,
			ParentTypeId:                       nil,
			SubTypeCreationPredicateSignatures: nil,
			SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
			TokenCreationPredicate:             script.PredicateAlwaysTrue(),
			InvariantPredicate:                 script.PredicateAlwaysTrue(),
		}
		_, err := tw.NewFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		tx, found := recTxs[string(typeId)]
		require.True(t, found)
		newFungibleTx := &ttxs.CreateFungibleTokenTypeAttributes{}
		require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newFungibleTx))
		require.Equal(t, typeId, tx.UnitId)
		require.Equal(t, a.Symbol, newFungibleTx.Symbol)
		require.Equal(t, a.DecimalPlaces, newFungibleTx.DecimalPlaces)
		require.EqualValues(t, tx.Timeout, 101)

		// new subtype
		b := &ttxs.CreateFungibleTokenTypeAttributes{
			Symbol:                             "AB",
			DecimalPlaces:                      2,
			ParentTypeId:                       typeId,
			SubTypeCreationPredicateSignatures: nil,
			SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
			TokenCreationPredicate:             script.PredicateAlwaysTrue(),
			InvariantPredicate:                 script.PredicateAlwaysTrue(),
		}
		//check decimal places are validated against the parent type
		_, err = tw.NewFungibleType(context.Background(), 1, b, []byte{2}, nil)
		require.ErrorContains(t, err, "parent type requires 0 decimal places, got 2")
	})

	t.Run("non-fungible type", func(t *testing.T) {
		typeId := test.RandomBytes(32)
		a := &ttxs.CreateNonFungibleTokenTypeAttributes{
			Symbol:                             "ABNFT",
			ParentTypeId:                       nil,
			SubTypeCreationPredicateSignatures: nil,
			SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
			TokenCreationPredicate:             script.PredicateAlwaysTrue(),
			InvariantPredicate:                 script.PredicateAlwaysTrue(),
		}
		_, err := tw.NewNonFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		tx, found := recTxs[string(typeId)]
		require.True(t, found)
		newNFTTx := &ttxs.CreateNonFungibleTokenTypeAttributes{}
		require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newNFTTx))
		require.Equal(t, typeId, tx.UnitId)
		require.Equal(t, a.Symbol, newNFTTx.Symbol)
	})
}

func TestMintFungibleToken(t *testing.T) {
	recTxs := make([]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
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
		name          string
		accNr         uint64
		validateOwner func(t *testing.T, accNr uint64, tok *ttxs.MintFungibleTokenAttributes)
	}{
		{
			name:  "pub key bearer predicate, account 1",
			accNr: uint64(1),
			validateOwner: func(t *testing.T, accNr uint64, tok *ttxs.MintFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:  "pub key bearer predicate, account 2",
			accNr: uint64(2),
			validateOwner: func(t *testing.T, accNr uint64, tok *ttxs.MintFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typeId := test.RandomBytes(32)
			amount := uint64(100)
			a := &ttxs.MintFungibleTokenAttributes{
				Type:                             typeId,
				Value:                            amount,
				TokenCreationPredicateSignatures: nil,
			}
			_, err := tw.NewFungibleToken(context.Background(), tt.accNr, a, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			newToken := &ttxs.MintFungibleTokenAttributes{}
			require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitId)
			require.Len(t, tx.UnitId, 32)
			require.Equal(t, typeId, newToken.Type)
			require.Equal(t, amount, newToken.Value)
			tt.validateOwner(t, tt.accNr, newToken)
		})
	}
}

func TestSendFungible(t *testing.T) {
	recTxs := make([]*txsystem.Transaction, 0)
	typeId := test.RandomBytes(32)
	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind twb.Kind, owner twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnit, string, error) {
			return []twb.TokenUnit{
				{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB", TypeID: typeId, Amount: 3},
				{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB", TypeID: typeId, Amount: 5},
				{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB", TypeID: typeId, Amount: 7},
				{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB", TypeID: typeId, Amount: 18},
			}, "", nil
		},
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
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
		attrs      *ttxs.MintNonFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "attributes missing",
			attrs:      nil,
			wantErrStr: "attributes missing",
		},
		{
			name: "invalid URI",
			attrs: &ttxs.MintNonFungibleTokenAttributes{
				Uri: "invalid_uri",
			},
			wantErrStr: "URI 'invalid_uri' is invalid",
		},
		{
			name: "URI exceeds maximum allowed length",
			attrs: &ttxs.MintNonFungibleTokenAttributes{
				Uri: string(test.RandomBytes(4097)),
			},
			wantErrStr: "URI exceeds the maximum allowed size of 4096 bytes",
		},
		{
			name: "data exceeds maximum allowed length",
			attrs: &ttxs.MintNonFungibleTokenAttributes{
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
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
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
		name          string
		accNr         uint64
		tokenID       twb.TokenID
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
			typeId := []byte{1}
			a := &ttxs.MintNonFungibleTokenAttributes{
				NftType:                          typeId,
				Uri:                              "https://alphabill.org",
				Data:                             nil,
				DataUpdatePredicate:              script.PredicateAlwaysTrue(),
				TokenCreationPredicateSignatures: nil,
			}
			_, err := tw.NewNFT(context.Background(), tt.accNr, a, tt.tokenID, nil)
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
	tokens := make(map[string]*twb.TokenUnit)

	recTxs := make(map[string]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		getToken: func(ctx context.Context, id twb.TokenID) (*twb.TokenUnit, error) {
			return tokens[string(id)], nil
		},
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitId)] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
	}
	tw := initTestWallet(t, be)

	first := func(s twb.PubKey, e error) twb.PubKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		token         *twb.TokenUnit
		key           twb.PubKey
		validateOwner func(t *testing.T, accNr uint64, key twb.PubKey, tok *ttxs.TransferNonFungibleTokenAttributes)
	}{
		{
			name:  "to 'always true' predicate",
			token: &twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)},
			key:   nil,
			validateOwner: func(t *testing.T, accNr uint64, key twb.PubKey, tok *ttxs.TransferNonFungibleTokenAttributes) {
				require.Equal(t, script.PredicateAlwaysTrue(), tok.NewBearer)
			},
		},
		{
			name:  "to public key hash predicate",
			token: &twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)},
			key:   first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accNr uint64, key twb.PubKey, tok *ttxs.TransferNonFungibleTokenAttributes) {
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
	tokens := make(map[string]*twb.TokenUnit)

	recTxs := make(map[string]*txsystem.Transaction, 0)
	be := &mockTokenBackend{
		getToken: func(ctx context.Context, id twb.TokenID) (*twb.TokenUnit, error) {
			return tokens[string(id)], nil
		},
		postTransactions: func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitId)] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
	}
	tw := initTestWallet(t, be)

	parseNFTDataUpdate := func(t *testing.T, tx *txsystem.Transaction) *ttxs.UpdateNonFungibleTokenAttributes {
		t.Helper()
		newTransfer := &ttxs.UpdateNonFungibleTokenAttributes{}
		require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
		return newTransfer
	}

	tok := &twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), TxHash: test.RandomBytes(32)}
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

func initTestWallet(t *testing.T, backend TokenBackend) *Wallet {
	t.Helper()
	txs, err := ttxs.New()
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
	getToken         func(ctx context.Context, id twb.TokenID) (*twb.TokenUnit, error)
	getTokens        func(ctx context.Context, kind twb.Kind, owner twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnit, string, error)
	getTokenTypes    func(ctx context.Context, kind twb.Kind, creator twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnitType, string, error)
	getRoundNumber   func(ctx context.Context) (uint64, error)
	postTransactions func(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error
	getTypeHierarchy func(ctx context.Context, id twb.TokenTypeID) ([]twb.TokenUnitType, error)
}

func (m *mockTokenBackend) GetToken(ctx context.Context, id twb.TokenID) (*twb.TokenUnit, error) {
	if m.getToken != nil {
		return m.getToken(ctx, id)
	}
	return nil, fmt.Errorf("GetToken not implemented")
}

func (m *mockTokenBackend) GetTokens(ctx context.Context, kind twb.Kind, owner twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnit, string, error) {
	if m.getTokens != nil {
		return m.getTokens(ctx, kind, owner, offsetKey, limit)
	}
	return nil, "", fmt.Errorf("GetTokens not implemented")
}

func (m *mockTokenBackend) GetTokenTypes(ctx context.Context, kind twb.Kind, creator twb.PubKey, offsetKey string, limit int) ([]twb.TokenUnitType, string, error) {
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

func (m *mockTokenBackend) PostTransactions(ctx context.Context, pubKey twb.PubKey, txs *txsystem.Transactions) error {
	if m.postTransactions != nil {
		return m.postTransactions(ctx, pubKey, txs)
	}
	return fmt.Errorf("PostTransactions not implemented")
}

func (m *mockTokenBackend) GetTypeHierarchy(ctx context.Context, id twb.TokenTypeID) ([]twb.TokenUnitType, error) {
	if m.getTypeHierarchy != nil {
		return m.getTypeHierarchy(ctx, id)
	}
	return nil, fmt.Errorf("GetTypeHierarchy not implemented")
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
