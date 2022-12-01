package tokens

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestNewFungibleType(t *testing.T) {
	tw, abClient := createTestWallet(t)
	typeId := []byte{1}
	a := &tokens.CreateFungibleTokenTypeAttributes{
		Symbol:                             "AB",
		DecimalPlaces:                      0,
		ParentTypeId:                       nil,
		SubTypeCreationPredicateSignatures: nil,
		SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
		TokenCreationPredicate:             script.PredicateAlwaysTrue(),
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
	}
	_, err := tw.NewFungibleType(context.Background(), a, typeId, nil)
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
		Symbol:                             "ABNFT",
		ParentTypeId:                       nil,
		SubTypeCreationPredicateSignatures: nil,
		SubTypeCreationPredicate:           script.PredicateAlwaysFalse(),
		TokenCreationPredicate:             script.PredicateAlwaysTrue(),
		InvariantPredicate:                 script.PredicateAlwaysTrue(),
	}
	_, err := tw.NewNonFungibleType(context.Background(), a, typeId, nil)
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
				Type:                             typeId,
				Value:                            amount,
				TokenCreationPredicateSignatures: nil,
			}
			_, err := tw.NewFungibleToken(context.Background(), tt.accNr, a, nil)
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

func TestMintNonFungibleToken_InvalidInputs(t *testing.T) {
	tokenID := test.RandomBytes(32)
	accNr := uint64(1)
	tests := []struct {
		name       string
		attrs      *tokens.MintNonFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "attributes missing",
			attrs:      nil,
			wantErrStr: "attributes missing",
		},
		{
			name: "invalid URI",
			attrs: &tokens.MintNonFungibleTokenAttributes{
				Uri: "invalid_uri",
			},
			wantErrStr: "URI 'invalid_uri' is invalid",
		},
		{
			name: "URI exceeds maximum allowed length",
			attrs: &tokens.MintNonFungibleTokenAttributes{
				Uri: string(test.RandomBytes(4097)),
			},
			wantErrStr: "URI exceeds the maximum allowed size of 4096 bytes",
		},
		{
			name: "data exceeds maximum allowed length",
			attrs: &tokens.MintNonFungibleTokenAttributes{
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
				NftType:                          typeId,
				Uri:                              "",
				Data:                             nil,
				DataUpdatePredicate:              script.PredicateAlwaysTrue(),
				TokenCreationPredicateSignatures: nil,
			}
			_, err := tw.NewNFT(context.Background(), tt.accNr, a, nil, nil)
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
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{11}, Kind: FungibleToken, Symbol: "AB", TypeID: []byte{10}, Amount: 1}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{12}, Kind: FungibleToken, Symbol: "AB", TypeID: []byte{10}, Amount: 2}))
		return nil
	})
	require.NoError(t, err)
	first := func(s PublicKey, e error) PublicKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		tokenId       TokenID
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
			require.NotEqual(t, tt.tokenId, tx.UnitId)
			newTransfer := parseFungibleTransfer(t, tx)
			require.Equal(t, tt.amount, newTransfer.Value)
			tt.validateOwner(t, 1, tt.key, newTransfer)
		})
	}
}

func TestTransferNFT(t *testing.T) {
	tw, abClient := createTestWallet(t)
	err := tw.db.WithTransaction(func(c TokenTxContext) error {
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{11}, Kind: NonFungibleToken, Symbol: "AB", TypeID: []byte{10}}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{12}, Kind: NonFungibleToken, Symbol: "AB", TypeID: []byte{10}}))
		return nil
	})
	require.NoError(t, err)
	first := func(s PublicKey, e error) PublicKey {
		require.NoError(t, e)
		return s
	}
	tests := []struct {
		name          string
		tokenId       TokenID
		key           PublicKey
		validateOwner func(t *testing.T, accNr uint64, key PublicKey, tok *tokens.TransferNonFungibleTokenAttributes)
	}{
		{
			name:    "to 'always true' predicate",
			tokenId: []byte{11},
			key:     nil,
			validateOwner: func(t *testing.T, accNr uint64, key PublicKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.Equal(t, script.PredicateAlwaysTrue(), tok.NewBearer)
			},
		},
		{
			name:    "to public key hash predicate",
			tokenId: []byte{12},
			key:     first(hexutil.Decode("0x0290a43bc454babf1ea8b0b76fcbb01a8f27a989047cf6d6d76397cc4756321e64")),
			validateOwner: func(t *testing.T, accNr uint64, key PublicKey, tok *tokens.TransferNonFungibleTokenAttributes) {
				require.Equal(t, script.PredicatePayToPublicKeyHashDefault(hash.Sum256(key)), tok.NewBearer)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err = tw.TransferNFT(context.Background(), 1, tt.tokenId, tt.key)
			require.NoError(t, err)
			txs := abClient.GetRecordedTransactions()
			tx := txs[len(txs)-1]
			require.NotEqual(t, tt.tokenId, tx.UnitId)
			newTransfer := parseNFTTransfer(t, tx)
			tt.validateOwner(t, 1, tt.key, newTransfer)
		})
	}
}

func parseFungibleTransfer(t *testing.T, tx *txsystem.Transaction) (newTransfer *tokens.TransferFungibleTokenAttributes) {
	newTransfer = &tokens.TransferFungibleTokenAttributes{}
	require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
	return
}

func parseNFTTransfer(t *testing.T, tx *txsystem.Transaction) (newTransfer *tokens.TransferNonFungibleTokenAttributes) {
	newTransfer = &tokens.TransferNonFungibleTokenAttributes{}
	require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
	return
}

func parseSplit(t *testing.T, tx *txsystem.Transaction) (newTransfer *tokens.SplitFungibleTokenAttributes) {
	newTransfer = &tokens.SplitFungibleTokenAttributes{}
	require.NoError(t, tx.TransactionAttributes.UnmarshalTo(newTransfer))
	return
}

func TestSendFungible(t *testing.T) {
	typeId := []byte{10}
	tw, abClient := createTestWallet(t)
	require.NoError(t, tw.db.WithTransaction(func(c TokenTxContext) error {
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{11}, Kind: FungibleToken, Symbol: "AB", TypeID: typeId, Amount: 3}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{12}, Kind: FungibleToken, Symbol: "AB", TypeID: typeId, Amount: 5}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{13}, Kind: FungibleToken, Symbol: "AB", TypeID: typeId, Amount: 7}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{14}, Kind: FungibleToken, Symbol: "AB", TypeID: typeId, Amount: 18}))
		return nil
	}))
	tests := []struct {
		name               string
		targetAmount       uint64
		expectedErrorMsg   string
		verifyTransactions func(t *testing.T, txs []*txsystem.Transaction)
	}{
		{
			name:         "one bill is transferred",
			targetAmount: 3,
			verifyTransactions: func(t *testing.T, txs []*txsystem.Transaction) {
				require.Equal(t, 1, len(txs))
				tx := txs[0]
				newTransfer := parseFungibleTransfer(t, tx)
				require.Equal(t, uint64(3), newTransfer.Value)
				require.Equal(t, []byte{11}, tx.UnitId)
			},
		},
		{
			name:         "one bill is split",
			targetAmount: 4,
			verifyTransactions: func(t *testing.T, txs []*txsystem.Transaction) {
				require.Equal(t, 1, len(txs))
				tx := txs[0]
				newSplit := parseSplit(t, tx)
				require.Equal(t, uint64(4), newSplit.TargetValue)
				require.Equal(t, []byte{12}, tx.UnitId)
			},
		},
		{
			name:         "both split and transfer are submitted",
			targetAmount: 26,
			verifyTransactions: func(t *testing.T, txs []*txsystem.Transaction) {
				var total = uint64(0)
				for _, tx := range txs {
					gtx, err := tw.txs.ConvertTx(tx)
					require.NoError(t, err)
					switch ctx := gtx.(type) {
					case tokens.TransferFungibleToken:
						total += ctx.Value()
					case tokens.SplitFungibleToken:
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
			abClient.ClearRecordedTransactions()
			err := tw.SendFungible(context.Background(), 1, typeId, tt.targetAmount, nil)
			if tt.expectedErrorMsg != "" {
				require.ErrorContains(t, err, tt.expectedErrorMsg)
				return
			} else {
				require.NoError(t, err)
			}
			tt.verifyTransactions(t, abClient.GetRecordedTransactions())
		})
	}
}

func TestList(t *testing.T) {
	tw, _ := createTestWallet(t)
	_, _, err := tw.mw.AddAccount() //#2
	require.NoError(t, err)
	_, _, err = tw.mw.AddAccount() //#3 this acc has no tokens, should not be listed
	require.NoError(t, err)
	require.NoError(t, tw.db.WithTransaction(func(c TokenTxContext) error {
		require.NoError(t, c.SetToken(0, &TokenUnit{ID: []byte{11}, TypeID: []byte{0x01}, Kind: FungibleToken, Symbol: "AB", Amount: 3}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{12}, TypeID: []byte{0x01}, Kind: FungibleToken, Symbol: "AB", Amount: 5}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{15}, TypeID: []byte{0x01}, Kind: FungibleToken, Symbol: "AB", Amount: 6}))
		require.NoError(t, c.SetToken(1, &TokenUnit{ID: []byte{13}, TypeID: []byte{0x02}, Kind: NonFungibleToken, Symbol: "AB", URI: "alphabill.org"}))
		require.NoError(t, c.SetToken(2, &TokenUnit{ID: []byte{14}, TypeID: []byte{0x01}, Kind: FungibleToken, Symbol: "AB", Amount: 18}))
		return nil
	}))
	countTotals := func(toks map[uint64][]*TokenUnit) (totalKeys int, totalTokens int) {
		for k, v := range toks {
			totalKeys++
			fmt.Printf("Key #%v\n", k)
			for _, tok := range v {
				totalTokens++
				fmt.Printf("Token=%s, amount=%v\n", tok.GetSymbol(), tok.Amount)
			}
		}
		return
	}
	tests := []struct {
		name      string
		accountNr int
		kind      TokenKind
		verify    func(t *testing.T, toks map[uint64][]*TokenUnit)
	}{
		{
			name:      "list all tokens across all accounts",
			accountNr: AllAccounts,
			kind:      Any,
			verify: func(t *testing.T, toks map[uint64][]*TokenUnit) {
				totalKeys, totalTokens := countTotals(toks)
				require.Equal(t, 3, totalKeys)
				require.Equal(t, 5, totalTokens)
			},
		}, {
			name:      "only tokens spendable by anyone",
			accountNr: 0,
			kind:      Any,
			verify: func(t *testing.T, toks map[uint64][]*TokenUnit) {
				totalKeys, totalTokens := countTotals(toks)
				require.Equal(t, 1, totalKeys)
				require.Equal(t, 1, totalTokens)
			},
		}, {
			name:      "account #1 only",
			accountNr: 1,
			kind:      Any,
			verify: func(t *testing.T, toks map[uint64][]*TokenUnit) {
				totalKeys, totalTokens := countTotals(toks)
				require.Equal(t, 1, totalKeys)
				require.Equal(t, 3, totalTokens)
			},
		}, {
			name:      "account #2 only",
			accountNr: 2,
			kind:      Any,
			verify: func(t *testing.T, toks map[uint64][]*TokenUnit) {
				totalKeys, totalTokens := countTotals(toks)
				require.Equal(t, 1, totalKeys)
				require.Equal(t, 1, totalTokens)
			},
		}, {
			name:      "account #3 only",
			accountNr: 3,
			kind:      Any,
			verify: func(t *testing.T, toks map[uint64][]*TokenUnit) {
				totalKeys, totalTokens := countTotals(toks)
				require.Equal(t, 0, totalKeys)
				require.Equal(t, 0, totalTokens)
			},
		}, {
			name:      "all accounts, only fungible",
			accountNr: AllAccounts,
			kind:      FungibleToken,
			verify: func(t *testing.T, toks map[uint64][]*TokenUnit) {
				totalKeys, totalTokens := countTotals(toks)
				require.Equal(t, 3, totalKeys)
				require.Equal(t, 4, totalTokens)
			},
		}, {
			name:      "all accounts, only non-fungible",
			accountNr: AllAccounts,
			kind:      NonFungibleToken,
			verify: func(t *testing.T, toks map[uint64][]*TokenUnit) {
				totalKeys, totalTokens := countTotals(toks)
				require.Equal(t, 1, totalKeys)
				require.Equal(t, 1, totalTokens)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tw.ListTokens(context.Background(), tt.kind, tt.accountNr)
			require.NoError(t, err)
			tt.verify(t, res)
		})
	}
}

func createTestWallet(t *testing.T) (*Wallet, *clientmock.MockAlphabillClient) {
	parentDir, err := ioutil.TempDir(os.TempDir(), "*-tests")
	require.NoError(t, err)
	c := money.WalletConfig{DbPath: parentDir}
	w, err := money.CreateNewWallet("", c)
	require.NoError(t, err)
	tw, err := Load(w, false)
	t.Cleanup(func() {
		deleteWallet(tw)
		os.RemoveAll(parentDir)
	})
	require.NoError(t, err)

	mockClient := clientmock.NewMockAlphabillClient(0, map[uint64]*block.Block{})
	w.AlphabillClient = mockClient
	return tw, mockClient
}

func deleteFile(dir string, file string) error {
	return os.Remove(path.Join(dir, file))
}

func deleteWallet(w *Wallet) {
	if w != nil {
		w.Shutdown()
		w.mw.DeleteDb()
		w.db.DeleteDb()
	}
}
