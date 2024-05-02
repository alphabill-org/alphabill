package tokens

import (
	"fmt"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/stretchr/testify/require"
)

func TestUnlockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txsystem.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))
	// create unlock tx
	unlockAttr := &tokens.UnlockTokenAttributes{
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	unlockTx := createTransactionOrder(t, unlockAttr, tokens.PayloadTypeUnlockToken, existingLockedTokenUnitID)
	roundNo := uint64(11)
	sm, err := txExecutors.ValidateAndExecute(unlockTx, &txsystem.TxExecutionContext{CurrentBlockNr: roundNo})
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedTokenUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	unitData := u.Data().(*tokens.FungibleTokenData)

	// verify token is unlocked, counter and round number is updated
	require.Equal(t, roundNo, unitData.T)
	require.Equal(t, uint64(1), unitData.Counter)
	require.EqualValues(t, 0, unitData.Locked)
}

func TestUnlockFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.UnlockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeUnlockToken, nil),
			attr:       &tokens.UnlockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeUnlockToken, existingTokenTypeUnitID),
			attr:       &tokens.UnlockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeUnlockToken, tokens.NewFungibleTokenID(nil, []byte{42})),
			attr:       &tokens.UnlockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already unlocked",
			tx: createTx(t, existingTokenUnitID, &tokens.UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{},
			}, tokens.PayloadTypeUnlockToken),
			wantErrStr: "token is already unlocked",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingLockedTokenUnitID, &tokens.UnlockTokenAttributes{
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{},
			}, tokens.PayloadTypeUnlockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingLockedTokenUnitID, &tokens.UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, tokens.PayloadTypeUnlockToken),
			wantErrStr: `token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			err := m.validateUnlockTokenTx(tt.tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestUnlockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txsystem.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))
	// create unlock tx
	unlockAttr := &tokens.UnlockTokenAttributes{
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	unlockTx := createTransactionOrder(t, unlockAttr, tokens.PayloadTypeUnlockToken, existingLockedNFTUnitID)
	roundNo := uint64(11)
	sm, err := txExecutors.ValidateAndExecute(unlockTx, &txsystem.TxExecutionContext{CurrentBlockNr: roundNo})
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	nftUnitData := u.Data().(*tokens.NonFungibleTokenData)

	// verify token is unlocked, counter and round number is updated
	require.Equal(t, roundNo, nftUnitData.T)
	require.Equal(t, uint64(1), nftUnitData.Counter)
	require.EqualValues(t, 0, nftUnitData.Locked)
}

func TestUnlockNFT_NotOk(t *testing.T) {
	// create state with existing NFT tokens
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}
	err := opts.state.Apply(state.AddUnit(existingNFTTypeUnitID, templates.AlwaysTrueBytes(), &tokens.NonFungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
		DataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingNFTUnitID, templates.AlwaysTrueBytes(), &tokens.NonFungibleTokenData{
		NftType:             existingNFTTypeUnitID,
		Name:                "ALPHA",
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingLockedNFTUnitID, templates.AlwaysTrueBytes(), &tokens.NonFungibleTokenData{
		NftType:             existingNFTTypeUnitID,
		Name:                "ALPHA",
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
		Locked:              1,
	}))
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.UnlockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeUnlockToken, nil),
			attr:       &tokens.UnlockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeUnlockToken, existingTokenTypeUnitID),
			attr:       &tokens.UnlockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeUnlockToken, tokens.NewNonFungibleTokenID(nil, []byte{42})),
			attr:       &tokens.UnlockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewNonFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already unlocked",
			tx: createTx(t, existingNFTUnitID, &tokens.UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{},
			}, tokens.PayloadTypeUnlockToken),
			wantErrStr: "token is already unlocked",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingLockedNFTUnitID, &tokens.UnlockTokenAttributes{
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{},
			}, tokens.PayloadTypeUnlockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingLockedNFTUnitID, &tokens.UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, tokens.PayloadTypeUnlockToken),
			wantErrStr: `token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}

	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			err := m.validateUnlockTokenTx(tt.tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}
