package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/api/types"
	sdk "github.com/alphabill-org/alphabill/client/wallet"
	"github.com/alphabill-org/alphabill/client/wallet/money/tx_builder"
	twb "github.com/alphabill-org/alphabill/client/wallet/tokens/backend"
	test "github.com/alphabill-org/alphabill/validator/pkg/testutils"
)

func TestFungibleTokenDC(t *testing.T) {
	am := initAccountManager(t)
	pubKey0, err := am.GetPublicKey(0)
	require.NoError(t, err)
	_, pubKey1, err := am.AddAccount()
	require.NoError(t, err)
	_, pubKey2, err := am.AddAccount()
	require.NoError(t, err)
	typeID1 := test.RandomBytes(32)
	typeID2 := test.RandomBytes(32)
	typeID3 := test.RandomBytes(32)

	resetFunc := func() (map[string]uint64, map[string][]*twb.TokenUnit, map[string]*types.TransactionOrder) {
		return map[string]uint64{
				string(pubKey0): 0,
				string(pubKey1): 0,
				string(pubKey2): 0,
			},
			map[string][]*twb.TokenUnit{
				string(pubKey0): {
					&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
					&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
					&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
					&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
				},
				string(pubKey1): {
					&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
				},
			},
			make(map[string]*types.TransactionOrder, 0)
	}

	burnedValues, accTokens, recordedTx := resetFunc()

	findToken := func(pubKey sdk.PubKey, id twb.TokenID) *twb.TokenUnit {
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

	removeToken := func(pubKey sdk.PubKey, id twb.TokenID) {
		tokens, found := accTokens[string(pubKey)]
		require.True(t, found, fmt.Sprintf("key %X not found", pubKey))
		// remove token
		for i, token := range tokens {
			if bytes.Equal(token.ID, id) {
				accTokens[string(pubKey)] = append(tokens[:i], tokens[i+1:]...)
				return
			}
		}
		t.Fatalf("unit %X not found", id)
	}

	be := &mockTokenBackend{
		getTokens: func(_ context.Context, _ twb.Kind, owner sdk.PubKey, _ string, _ int) ([]*twb.TokenUnit, string, error) {
			tokens, found := accTokens[string(owner)]
			if !found {
				return nil, "", nil
			}
			return tokens, "", nil
		},
		postTransactions: func(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
			for _, tx := range txs.Transactions {
				unitID := tx.UnitID()
				recordedTx[string(unitID)] = tx
				switch tx.PayloadType() {
				case tokens.PayloadTypeBurnFungibleToken:
					tok := findToken(pubKey, unitID)
					tok.Burned = true
					burnedValues[string(pubKey)] += tok.Amount
					removeToken(pubKey, unitID)
				case tokens.PayloadTypeJoinFungibleToken:
					tok := findToken(pubKey, unitID)
					attrs := &tokens.JoinFungibleTokenAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					tok.Amount += burnedValues[string(pubKey)]
					if bytes.Equal(pubKey1, pubKey) {
						require.Equal(t, uint64(300), tok.Amount)
					}
					for i := 1; i < len(attrs.BurnTransactions); i++ {
						prevID := attrs.BurnTransactions[i-1].TransactionOrder.UnitID()
						currID := attrs.BurnTransactions[i].TransactionOrder.UnitID()
						require.Equal(t, 1, currID.Compare(prevID), "invalid burn txs order in join tx")
					}
				default:
					return errors.New("unexpected tx")
				}
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
			recordedTx, found := recordedTx[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &sdk.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: recordedTx, ServerMetadata: &types.ServerMetadata{ActualFee: tx_builder.MaxFee}}, TxProof: nil}, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID types.UnitID) (*sdk.Bill, error) {
			return &sdk.Bill{
				Id:     []byte{1},
				Value:  100000,
				TxHash: []byte{2},
			}, nil
		},
	}
	tw := initTestWallet(t, be)
	tw.am = am

	ctx := context.Background()

	// this should only join tokens with type typeID3
	results, err := tw.CollectDust(ctx, AllAccounts, nil, nil)
	require.NoError(t, err)
	require.Len(t, recordedTx, 3) // 2 burns, 1 join
	require.Len(t, results, 1)
	require.Equal(t, uint64(3), results[0].FeeSum)
	// tx validation is done in postTransactions()

	// repeat, but with the specific account number
	burnedValues, accTokens, recordedTx = resetFunc()
	results, err = tw.CollectDust(ctx, 1, nil, nil)
	require.NoError(t, err)
	require.Len(t, recordedTx, 3) // 2 burns, 1 join
	require.Len(t, results, 1)
	require.Equal(t, uint64(3), results[0].FeeSum)

	// DC amount uint64 overflow, single batch
	burnedValues, accTokens, recordedTx = resetFunc()
	accTokens[string(pubKey2)] = []*twb.TokenUnit{
		{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID1, Amount: math.MaxUint64},
		{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID1, Amount: 1},
	}
	results, err = tw.CollectDust(ctx, 3, nil, nil)
	require.NoError(t, err)
	require.Empty(t, recordedTx, "no tx should be recorded if uint64 overflow occurs")
	require.Empty(t, results)

	// DC amount uint64 overflow, two batches, second one causes overflow
	burnedValues, accTokens, recordedTx = resetFunc()
	expectedToJoinInFirstBatch := uint64(0)
	for i := 0; i <= maxBurnBatchSize; i++ {
		token := &twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID1, Amount: uint64(i + 1)}
		accTokens[string(pubKey2)] = append(accTokens[string(pubKey2)], token)
		expectedToJoinInFirstBatch += token.Amount
	}
	accTokens[string(pubKey2)] = append(accTokens[string(pubKey2)], &twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID1, Amount: math.MaxUint64})
	results, err = tw.CollectDust(ctx, 3, nil, nil)
	require.NoError(t, err)
	require.Len(t, recordedTx, 101)               // 100 burns, 1 join from the first batch
	require.Len(t, accTokens[string(pubKey2)], 2) // 1 token joined and 1 left from the second batch
	require.Equal(t, expectedToJoinInFirstBatch, accTokens[string(pubKey2)][0].Amount)
	require.EqualValues(t, uint64(math.MaxUint64), accTokens[string(pubKey2)][1].Amount)
	require.Len(t, results, 1)
	require.Equal(t, uint64(101), results[0].FeeSum)
}

func TestGetTokensForDC(t *testing.T) {
	typeID1 := test.RandomBytes(32)
	typeID2 := test.RandomBytes(32)
	typeID3 := test.RandomBytes(32)

	allTokens := []*twb.TokenUnit{
		{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
		{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
		{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
		{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
		{ID: test.RandomBytes(32), Kind: twb.NonFungible, Symbol: "AB3", TypeID: typeID3},
	}

	be := &mockTokenBackend{
		getTokens: func(_ context.Context, kind twb.Kind, owner sdk.PubKey, _ string, _ int) ([]*twb.TokenUnit, string, error) {
			require.Equal(t, twb.Fungible, kind)
			var res []*twb.TokenUnit
			for _, tok := range allTokens {
				if tok.Kind != kind {
					continue
				}
				res = append(res, tok)
			}
			return res, "", nil
		},
	}
	tw := initTestWallet(t, be)
	key, err := tw.GetAccountManager().GetPublicKey(0)
	require.NoError(t, err)

	tests := []struct {
		allowedTypes []twb.TokenTypeID
		expected     map[string][]*twb.TokenUnit
	}{
		{
			allowedTypes: nil,
			expected:     map[string][]*twb.TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: make([]twb.TokenTypeID, 0),
			expected:     map[string][]*twb.TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []twb.TokenTypeID{test.RandomBytes(32)},
			expected:     map[string][]*twb.TokenUnit{},
		},
		{
			allowedTypes: []twb.TokenTypeID{typeID3},
			expected:     map[string][]*twb.TokenUnit{},
		},
		{
			allowedTypes: []twb.TokenTypeID{typeID1},
			expected:     map[string][]*twb.TokenUnit{string(typeID1): allTokens[:2]},
		},
		{
			allowedTypes: []twb.TokenTypeID{typeID2},
			expected:     map[string][]*twb.TokenUnit{string(typeID2): allTokens[2:4]},
		},
		{
			allowedTypes: []twb.TokenTypeID{typeID1, typeID2},
			expected:     map[string][]*twb.TokenUnit{string(typeID1): allTokens[:2], string(typeID2): allTokens[2:4]},
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
