package tokens

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/stretchr/testify/require"
)

func TestFungibleTokenDC(t *testing.T) {
	am := initAccountManager(t)
	pubKey0, err := am.GetPublicKey(0)
	require.NoError(t, err)
	_, pubKey1, err := am.AddAccount()
	require.NoError(t, err)
	typeID1 := test.RandomBytes(32)
	typeID2 := test.RandomBytes(32)
	typeID3 := test.RandomBytes(32)

	resetFunc := func() (uint64, map[string][]*twb.TokenUnit, map[string]*types.TransactionOrder) {
		return uint64(0), map[string][]*twb.TokenUnit{
			string(pubKey0): {
				&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB1", TypeID: typeID1, Amount: 100},
				&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
				&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
				&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB3", TypeID: typeID3, Amount: 100},
			},
			string(pubKey1): {
				&twb.TokenUnit{ID: test.RandomBytes(32), Kind: twb.Fungible, Symbol: "AB2", TypeID: typeID2, Amount: 100},
			},
		}, make(map[string]*types.TransactionOrder, 0)
	}

	burnedValue, accTokens, recordedTx := resetFunc()

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

	be := &mockTokenBackend{
		getTokens: func(_ context.Context, _ twb.Kind, owner sdk.PubKey, _ string, _ int) ([]twb.TokenUnit, string, error) {
			tokens, found := accTokens[string(owner)]
			if !found {
				return nil, "", fmt.Errorf("no tokens for pubkey '%X'", owner)
			}
			var res []twb.TokenUnit
			for _, tok := range tokens {
				res = append(res, *tok)
			}
			return res, "", nil
		},
		postTransactions: func(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
			for _, tx := range txs.Transactions {
				unitID := tx.UnitID()
				recordedTx[string(unitID)] = tx
				switch tx.PayloadType() {
				case ttxs.PayloadTypeBurnFungibleToken:
					tok := findToken(pubKey, unitID)
					tok.Burned = true
					burnedValue += tok.Amount
				case ttxs.PayloadTypeJoinFungibleToken:
					tok := findToken(pubKey, unitID)
					attrs := &ttxs.JoinFungibleTokenAttributes{}
					require.NoError(t, tx.UnmarshalAttributes(attrs))
					require.Equal(t, uint64(300), tok.Amount+burnedValue)
				default:
					return errors.New("unexpected tx")
				}
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID sdk.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
			recordedTx, found := recordedTx[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &sdk.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: recordedTx}, TxProof: nil}, nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 1, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID sdk.UnitID) (*twb.FeeCreditBill, error) {
			return &twb.FeeCreditBill{
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

	// repeat, but with the specific account number
	burnedValue, accTokens, recordedTx = resetFunc()
	require.NoError(t, tw.CollectDust(ctx, 1, nil, nil))
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
		getTokens: func(_ context.Context, kind twb.Kind, owner sdk.PubKey, _ string, _ int) ([]twb.TokenUnit, string, error) {
			require.Equal(t, twb.Fungible, kind)
			var res []twb.TokenUnit
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
