package tokens

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
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

	w, err := New(ttxs.DefaultSystemIdentifier, srv.URL, nil, false, nil, logger.New(t))
	require.NoError(t, err)

	rn, err := w.GetRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 42, rn)
}

func Test_ListTokens(t *testing.T) {
	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind backend.Kind, _ wallet.PubKey, _ string, _ int) ([]*backend.TokenUnit, string, error) {
			fungible := []*backend.TokenUnit{
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
			}
			nfts := []*backend.TokenUnit{
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
	allTokens := []*backend.TokenUnit{
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
		getTokens: func(ctx context.Context, kind backend.Kind, _ wallet.PubKey, offset string, _ int) ([]*backend.TokenUnit, string, error) {
			return getSubarray(allTokens, offset)
		},
	}

	tw := initTestWallet(t, be)
	tokens, err := tw.ListTokens(context.Background(), backend.Any, AllAccounts)
	tokensForAccount := tokens[1]
	require.NoError(t, err)
	require.Len(t, tokensForAccount, len(allTokens))
	require.Equal(t, allTokens, tokensForAccount)
}

func Test_ListTokenTypes(t *testing.T) {
	var firstPubKey *wallet.PubKey
	be := &mockTokenBackend{
		getTokenTypes: func(ctx context.Context, kind backend.Kind, pubKey wallet.PubKey, _ string, _ int) ([]*backend.TokenUnitType, string, error) {
			if !bytes.Equal(pubKey, *firstPubKey) {
				return []*backend.TokenUnitType{}, "", nil
			}

			fungible := []*backend.TokenUnitType{
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
				{
					ID:   test.RandomBytes(32),
					Kind: backend.Fungible,
				},
			}
			nfts := []*backend.TokenUnitType{
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
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)
	firstPubKey = (*wallet.PubKey)(&key)

	types, err := tw.ListTokenTypes(context.Background(), 0, backend.Any)
	require.NoError(t, err)
	require.Len(t, types, 4)

	types, err = tw.ListTokenTypes(context.Background(), 0, backend.Fungible)
	require.NoError(t, err)
	require.Len(t, types, 2)

	types, err = tw.ListTokenTypes(context.Background(), 0, backend.NonFungible)
	require.NoError(t, err)
	require.Len(t, types, 2)

	_, err = tw.ListTokenTypes(context.Background(), 2, backend.NonFungible)
	require.ErrorContains(t, err, "account does not exist")

	_, _, err = tw.am.AddAccount()
	require.NoError(t, err)

	types, err = tw.ListTokenTypes(context.Background(), 2, backend.Any)
	require.NoError(t, err)
	require.Len(t, types, 0)
}

func Test_ListTokenTypes_offset(t *testing.T) {
	allTypes := []*backend.TokenUnitType{
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
		getTokenTypes: func(ctx context.Context, _ backend.Kind, _ wallet.PubKey, offset string, _ int) ([]*backend.TokenUnitType, string, error) {
			return getSubarray(allTypes, offset)
		},
	}

	tw := initTestWallet(t, be)
	types, err := tw.ListTokenTypes(context.Background(), 0, backend.Any)
	require.NoError(t, err)
	require.Len(t, types, len(allTypes))
	require.Equal(t, allTypes, types)
}

func TestNewTypes(t *testing.T) {
	t.Parallel()

	recTxs := make(map[string]*types.TransactionOrder, 0)
	be := &mockTokenBackend{
		getTypeHierarchy: func(ctx context.Context, id backend.TokenTypeID) ([]*backend.TokenUnitType, error) {
			tx, found := recTxs[string(id)]
			if found {
				tokenType := &backend.TokenUnitType{ID: tx.UnitID()}
				if tx.PayloadType() == ttxs.PayloadTypeCreateFungibleTokenType {
					tokenType.Kind = backend.Fungible
					attrs := &ttxs.CreateFungibleTokenTypeAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeID
					tokenType.DecimalPlaces = attrs.DecimalPlaces
				} else {
					tokenType.Kind = backend.NonFungible
					attrs := &ttxs.CreateNonFungibleTokenTypeAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					tokenType.ParentTypeID = attrs.ParentTypeID
				}
				return []*backend.TokenUnitType{tokenType}, nil
			}
			return nil, fmt.Errorf("not found")
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
			return &wallet.Bill{
				Id:     []byte{1},
				Value:  100000,
				TxHash: []byte{2},
			}, nil
		},
	}
	tw := initTestWallet(t, be)

	t.Run("fungible type", func(t *testing.T) {
		typeId := ttxs.NewFungibleTokenTypeID(nil, test.RandomBytes(32))
		a := CreateFungibleTokenTypeAttributes{
			Symbol:                   "AB",
			Name:                     "Long name for AB",
			Icon:                     &Icon{Type: "image/png", Data: []byte{1}},
			DecimalPlaces:            0,
			ParentTypeId:             nil,
			SubTypeCreationPredicate: wallet.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   wallet.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       wallet.Predicate(templates.AlwaysTrueBytes()),
		}
		result, err := tw.NewFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeId, result.TokenTypeID)
		tx, found := recTxs[string(typeId)]
		require.True(t, found)
		newFungibleTx := &ttxs.CreateFungibleTokenTypeAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newFungibleTx))
		require.Equal(t, typeId, tx.UnitID())
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
			SubTypeCreationPredicate: wallet.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   wallet.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       wallet.Predicate(templates.AlwaysTrueBytes()),
		}
		//check decimal places are validated against the parent type
		_, err = tw.NewFungibleType(context.Background(), 1, b, nil, nil)
		require.ErrorContains(t, err, "parent type requires 0 decimal places, got 2")

		//check typeId length validation
		_, err = tw.NewFungibleType(context.Background(), 1, a, []byte{2}, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

		//check typeId unit type validation
		_, err = tw.NewFungibleType(context.Background(), 1, a, make([]byte, ttxs.UnitIDLength), nil)
		require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x20")

		//check typeId generation if typeId parameter is nil
		result, _ = tw.NewFungibleType(context.Background(), 1, a, nil, nil)
		require.True(t, result.TokenTypeID.HasType(ttxs.FungibleTokenTypeUnitType))
	})

	t.Run("non-fungible type", func(t *testing.T) {
		typeId := ttxs.NewNonFungibleTokenTypeID(nil, test.RandomBytes(32))
		a := CreateNonFungibleTokenTypeAttributes{
			Symbol:                   "ABNFT",
			Name:                     "Long name for ABNFT",
			Icon:                     &Icon{Type: "image/svg", Data: []byte{2}},
			ParentTypeId:             nil,
			SubTypeCreationPredicate: wallet.Predicate(templates.AlwaysFalseBytes()),
			TokenCreationPredicate:   wallet.Predicate(templates.AlwaysTrueBytes()),
			InvariantPredicate:       wallet.Predicate(templates.AlwaysTrueBytes()),
		}
		result, err := tw.NewNonFungibleType(context.Background(), 1, a, typeId, nil)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.EqualValues(t, typeId, result.TokenTypeID)
		tx, found := recTxs[string(typeId)]
		require.True(t, found)
		newNFTTx := &ttxs.CreateNonFungibleTokenTypeAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newNFTTx))
		require.Equal(t, typeId, tx.UnitID())
		require.Equal(t, a.Symbol, newNFTTx.Symbol)
		require.Equal(t, a.Icon.Type, newNFTTx.Icon.Type)
		require.Equal(t, a.Icon.Data, newNFTTx.Icon.Data)

		//check typeId length validation
		_, err = tw.NewNonFungibleType(context.Background(), 1, a, []byte{2}, nil)
		require.ErrorContains(t, err, "invalid token type ID: expected hex length is 66 characters (33 bytes)")

		//check typeId unit type validation
		_, err = tw.NewNonFungibleType(context.Background(), 1, a, make([]byte, ttxs.UnitIDLength), nil)
		require.ErrorContains(t, err, "invalid token type ID: expected unit type is 0x22")

		//check typeId generation if typeId parameter is nil
		result, _ = tw.NewNonFungibleType(context.Background(), 1, a, nil, nil)
		require.True(t, result.TokenTypeID.HasType(ttxs.NonFungibleTokenTypeUnitType))
	})
}

func TestMintFungibleToken(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	be := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recTxs = append(recTxs, txs.Transactions...)
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
			return &wallet.Bill{
				Id:     []byte{1},
				Value:  100000,
				TxHash: []byte{2},
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
			typeId := test.RandomBytes(33)
			amount := uint64(100)
			key, err := tw.am.GetAccountKey(tt.accNr - 1)
			require.NoError(t, err)
			result, err := tw.NewFungibleToken(context.Background(), tt.accNr, typeId, amount, bearerPredicateFromHash(key.PubKeyHash.Sha256), nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			newToken := &ttxs.MintFungibleTokenAttributes{}
			require.NotNil(t, result)
			require.EqualValues(t, tx.UnitID(), result.TokenID)
			require.NoError(t, tx.UnmarshalAttributes(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitID())
			require.Len(t, tx.UnitID(), 33)
			require.EqualValues(t, typeId, newToken.TypeID)
			require.Equal(t, amount, newToken.Value)
			require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), newToken.Bearer)
		})
	}
}

func TestSendFungible(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	typeId := test.RandomBytes(32)
	typeIdForOverflow := test.RandomBytes(32)
	be := &mockTokenBackend{
		getTokens: func(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offset string, limit int) ([]*backend.TokenUnit, string, error) {
			return []*backend.TokenUnit{
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 3},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 5},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 7},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB", TypeID: typeId, Amount: 18},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB2", TypeID: typeIdForOverflow, Amount: math.MaxUint64},
				{ID: test.RandomBytes(32), Kind: backend.Fungible, Symbol: "AB2", TypeID: typeIdForOverflow, Amount: 1},
			}, "", nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
			return &wallet.Bill{
				Id:     []byte{1},
				Value:  100000,
				TxHash: []byte{2},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
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
		tokenTypeID        backend.TokenTypeID
		targetAmount       uint64
		expectedErrorMsg   string
		verifyTransactions func(t *testing.T)
	}{
		{
			name:         "one bill is transferred",
			tokenTypeID:  typeId,
			targetAmount: 3,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newTransfer := &ttxs.TransferFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newTransfer))
				require.Equal(t, uint64(3), newTransfer.Value)
			},
		},
		{
			name:         "one bill is split",
			tokenTypeID:  typeId,
			targetAmount: 4,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newSplit := &ttxs.SplitFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newSplit))
				require.Equal(t, uint64(4), newSplit.TargetValue)
			},
		},
		{
			name:         "both split and transfer are submitted",
			tokenTypeID:  typeId,
			targetAmount: 26,
			verifyTransactions: func(t *testing.T) {
				var total = uint64(0)
				for _, tx := range recTxs {
					switch tx.PayloadType() {
					case ttxs.PayloadTypeTransferFungibleToken:
						attrs := &ttxs.TransferFungibleTokenAttributes{}
						require.NoError(t, tx.UnmarshalAttributes(attrs))
						total += attrs.GetValue()
					case ttxs.PayloadTypeSplitFungibleToken:
						attrs := &ttxs.SplitFungibleTokenAttributes{}
						require.NoError(t, tx.UnmarshalAttributes(attrs))
						total += attrs.GetTargetValue()
					default:
						t.Errorf("unexpected tx type: %s", tx.PayloadType())
					}
				}
				require.Equal(t, uint64(26), total)
			},
		},
		{
			name:             "insufficient balance",
			tokenTypeID:      typeId,
			targetAmount:     60,
			expectedErrorMsg: "insufficient value",
		},
		{
			name:             "zero amount",
			tokenTypeID:      typeId,
			targetAmount:     0,
			expectedErrorMsg: "invalid amount",
		},
		{
			name:         "total balance uint64 overflow, transfer is submitted",
			tokenTypeID:  typeIdForOverflow,
			targetAmount: 1,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newTransfer := &ttxs.TransferFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newTransfer))
				require.Equal(t, uint64(1), newTransfer.Value)
			},
		},
		{
			name:         "total balance uint64 overflow, transfer is submitted with maxuint64",
			tokenTypeID:  typeIdForOverflow,
			targetAmount: math.MaxUint64,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newTransfer := &ttxs.TransferFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newTransfer))
				require.Equal(t, uint64(math.MaxUint64), newTransfer.Value)
			},
		},
		{
			name:         "total balance uint64 overflow, split is submitted",
			tokenTypeID:  typeIdForOverflow,
			targetAmount: 2,
			verifyTransactions: func(t *testing.T) {
				require.Equal(t, 1, len(recTxs))
				tx := recTxs[0]
				newSplit := &ttxs.SplitFungibleTokenAttributes{}
				require.NoError(t, tx.UnmarshalAttributes(newSplit))
				require.Equal(t, uint64(2), newSplit.TargetValue)
				require.Equal(t, uint64(math.MaxUint64-2), newSplit.RemainingValue)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recTxs = make([]*types.TransactionOrder, 0)
			result, err := tw.SendFungible(context.Background(), 1, tt.tokenTypeID, tt.targetAmount, nil, nil)
			if tt.expectedErrorMsg != "" {
				require.ErrorContains(t, err, tt.expectedErrorMsg)
				return
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
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
			wallet := &Wallet{log: logger.New(t)}
			got, err := wallet.NewNFT(context.Background(), accNr, tt.attrs, tokenID, nil)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, got)
		})
	}
}

func TestMintNFT(t *testing.T) {
	recTxs := make([]*types.TransactionOrder, 0)
	be := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recTxs = append(recTxs, txs.Transactions...)
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
			return &wallet.Bill{
				Id:     []byte{1},
				Value:  100000,
				TxHash: []byte{2},
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
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:    "pub key bearer predicate, account 1, predefined token ID",
			accNr:   uint64(1),
			tokenID: test.RandomBytes(33),
			validateOwner: func(t *testing.T, accNr uint64, tok *ttxs.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
			},
		},
		{
			name:  "pub key bearer predicate, account 2",
			accNr: uint64(2),
			validateOwner: func(t *testing.T, accNr uint64, tok *ttxs.MintNonFungibleTokenAttributes) {
				key, err := tw.am.GetAccountKey(accNr - 1)
				require.NoError(t, err)
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(key.PubKeyHash.Sha256), tok.Bearer)
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
				DataUpdatePredicate: wallet.Predicate(templates.AlwaysTrueBytes()),
			}
			result, err := tw.NewNFT(context.Background(), tt.accNr, a, tt.tokenID, nil)
			require.NoError(t, err)
			tx := recTxs[len(recTxs)-1]
			newToken := &ttxs.MintNonFungibleTokenAttributes{}
			require.NotNil(t, result)
			require.EqualValues(t, tx.UnitID(), result.TokenID)
			require.NoError(t, tx.UnmarshalAttributes(newToken))
			require.NotEqual(t, []byte{0}, tx.UnitID())
			require.Len(t, tx.UnitID(), 33)
			if tt.tokenID != nil {
				require.EqualValues(t, tt.tokenID, tx.UnitID())
			}
			require.EqualValues(t, typeId, newToken.NFTTypeID)
			tt.validateOwner(t, tt.accNr, newToken)
		})
	}
}

func TestTransferNFT(t *testing.T) {
	tokens := make(map[string]*backend.TokenUnit)

	recTxs := make(map[string]*types.TransactionOrder, 0)
	be := &mockTokenBackend{
		getToken: func(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error) {
			return tokens[string(id)], nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
			return &wallet.Bill{
				Id:     []byte{1},
				Value:  100000,
				TxHash: []byte{2},
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
				require.EqualValues(t, templates.AlwaysTrueBytes(), tok.NewBearer)
			},
		},
		{
			name:  "to public key hash predicate",
			token: &backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32)},
			key:   first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accNr uint64, key wallet.PubKey, tok *ttxs.TransferNonFungibleTokenAttributes) {
				require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(key)), tok.NewBearer)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens[string(tt.token.ID)] = tt.token
			result, err := tw.TransferNFT(context.Background(), 1, tt.token.ID, tt.key, nil)
			require.NoError(t, err)
			require.NotNil(t, result)
			tx, found := recTxs[string(tt.token.ID)]
			require.True(t, found)
			require.EqualValues(t, tt.token.ID, tx.UnitID())
			newTransfer := &ttxs.TransferNonFungibleTokenAttributes{}
			require.NoError(t, tx.UnmarshalAttributes(newTransfer))
			tt.validateOwner(t, 1, tt.key, newTransfer)
		})
	}
}

func TestUpdateNFTData(t *testing.T) {
	tokens := make(map[string]*backend.TokenUnit)

	recTxs := make(map[string]*types.TransactionOrder, 0)
	be := &mockTokenBackend{
		getToken: func(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error) {
			return tokens[string(id)], nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
			return &wallet.Bill{
				Id:     []byte{1},
				Value:  100000,
				TxHash: []byte{2},
			}, nil
		},
	}
	tw := initTestWallet(t, be)

	parseNFTDataUpdate := func(t *testing.T, tx *types.TransactionOrder) *ttxs.UpdateNonFungibleTokenAttributes {
		t.Helper()
		newTransfer := &ttxs.UpdateNonFungibleTokenAttributes{}
		require.NoError(t, tx.UnmarshalAttributes(newTransfer))
		return newTransfer
	}

	tok := &backend.TokenUnit{ID: test.RandomBytes(32), Kind: backend.NonFungible, Symbol: "AB", TypeID: test.RandomBytes(32), TxHash: test.RandomBytes(32)}
	tokens[string(tok.ID)] = tok

	// test data, backlink and predicate inputs are submitted correctly
	data := test.RandomBytes(64)
	result, err := tw.UpdateNFTData(context.Background(), 1, tok.ID, data, []*PredicateInput{{Argument: nil}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found := recTxs[string(tok.ID)]
	require.True(t, found)

	dataUpdate := parseNFTDataUpdate(t, tx)
	require.Equal(t, data, dataUpdate.Data)
	require.EqualValues(t, tok.TxHash, dataUpdate.Backlink)
	require.Equal(t, [][]byte{nil}, dataUpdate.DataUpdateSignatures)

	// test that wallet not only sends the tx, but also reads it correctly
	data2 := test.RandomBytes(64)
	result, err = tw.UpdateNFTData(context.Background(), 1, tok.ID, data2, []*PredicateInput{{Argument: nil}, {AccountNumber: 1}})
	require.NoError(t, err)
	require.NotNil(t, result)
	tx, found = recTxs[string(tok.ID)]
	require.True(t, found)
	dataUpdate = parseNFTDataUpdate(t, tx)
	require.NotEqual(t, data, dataUpdate.Data)
	require.Equal(t, data2, dataUpdate.Data)
	require.Len(t, dataUpdate.DataUpdateSignatures, 2)
	require.Equal(t, []byte(nil), dataUpdate.DataUpdateSignatures[0])
	require.Len(t, dataUpdate.DataUpdateSignatures[1], 103)
}

func initTestWallet(t *testing.T, backend TokenBackend) *Wallet {
	t.Helper()
	return &Wallet{
		am:      initAccountManager(t),
		backend: backend,
		log:     logger.New(t),
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
	getTokens        func(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offset string, limit int) ([]*backend.TokenUnit, string, error)
	getTokenTypes    func(ctx context.Context, kind backend.Kind, creator wallet.PubKey, offset string, limit int) ([]*backend.TokenUnitType, string, error)
	getRoundNumber   func(ctx context.Context) (uint64, error)
	postTransactions func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
	getTypeHierarchy func(ctx context.Context, id backend.TokenTypeID) ([]*backend.TokenUnitType, error)
	getTxProof       func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	getFeeCreditBill func(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error)
}

func (m *mockTokenBackend) GetToken(ctx context.Context, id backend.TokenID) (*backend.TokenUnit, error) {
	if m.getToken != nil {
		return m.getToken(ctx, id)
	}
	return nil, fmt.Errorf("GetToken not implemented")
}

func (m *mockTokenBackend) GetTokens(ctx context.Context, kind backend.Kind, owner wallet.PubKey, offset string, limit int) ([]*backend.TokenUnit, string, error) {
	if m.getTokens != nil {
		return m.getTokens(ctx, kind, owner, offset, limit)
	}
	return nil, "", fmt.Errorf("GetTokens not implemented")
}

func (m *mockTokenBackend) GetTokenTypes(ctx context.Context, kind backend.Kind, creator wallet.PubKey, offset string, limit int) ([]*backend.TokenUnitType, string, error) {
	if m.getTokenTypes != nil {
		return m.getTokenTypes(ctx, kind, creator, offset, limit)
	}
	return nil, "", fmt.Errorf("GetTokenTypes not implemented")
}

func (m *mockTokenBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	if m.getRoundNumber != nil {
		return m.getRoundNumber(ctx)
	}
	return 0, fmt.Errorf("GetRoundNumber not implemented")
}

func (m *mockTokenBackend) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
	if m.postTransactions != nil {
		return m.postTransactions(ctx, pubKey, txs)
	}
	return fmt.Errorf("PostTransactions not implemented")
}

func (m *mockTokenBackend) GetTypeHierarchy(ctx context.Context, id backend.TokenTypeID) ([]*backend.TokenUnitType, error) {
	if m.getTypeHierarchy != nil {
		return m.getTypeHierarchy(ctx, id)
	}
	return nil, fmt.Errorf("GetTypeHierarchy not implemented")
}

func (m *mockTokenBackend) GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	if m.getTxProof != nil {
		return m.getTxProof(ctx, unitID, txHash)
	}
	return nil, fmt.Errorf("GetTxProof not implemented")
}

func (m *mockTokenBackend) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
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
	subarray := array[offset:min(offset+defaultLimit, len(array))]
	offset += defaultLimit
	if offset >= len(array) {
		offsetKey = ""
	} else {
		offsetKey = strconv.Itoa(offset)
	}
	return subarray, offsetKey, nil
}
