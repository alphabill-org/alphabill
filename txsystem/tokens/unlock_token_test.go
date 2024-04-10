package tokens

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func TestUnlockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	// create unlock tx
	unlockAttr := &UnlockTokenAttributes{
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	unlockTx := createTransactionOrder(t, unlockAttr, PayloadTypeUnlockToken, existingLockedTokenUnitID)
	roundNo := uint64(11)
	sm, err := m.handleUnlockTokenTx()(unlockTx, unlockAttr, &txsystem.TxExecutionContext{CurrentBlockNr: roundNo})
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedTokenUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &FungibleTokenData{}, u.Data())
	unitData := u.Data().(*FungibleTokenData)

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
		attr       *UnlockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockToken, nil),
			attr:       &UnlockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockToken, existingTokenTypeUnitID),
			attr:       &UnlockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockToken, NewFungibleTokenID(nil, []byte{42})),
			attr:       &UnlockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already unlocked",
			tx: createTx(t, existingTokenUnitID, &UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "token is already unlocked",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingLockedTokenUnitID, &UnlockTokenAttributes{
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingLockedTokenUnitID, &UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeUnlockToken),
			wantErrStr: `invalid unlock token tx: token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := m.handleUnlockTokenTx()(tt.tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestUnlockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	// create unlock tx
	unlockAttr := &UnlockTokenAttributes{
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	unlockTx := createTransactionOrder(t, unlockAttr, PayloadTypeUnlockToken, existingLockedNFTUnitID)
	roundNo := uint64(11)
	sm, err := m.handleUnlockTokenTx()(unlockTx, unlockAttr, &txsystem.TxExecutionContext{CurrentBlockNr: roundNo})
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &NonFungibleTokenData{}, u.Data())
	nftUnitData := u.Data().(*NonFungibleTokenData)

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
	err := opts.state.Apply(state.AddUnit(existingNFTTypeUnitID, templates.AlwaysTrueBytes(), &NonFungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
		DataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingNFTUnitID, templates.AlwaysTrueBytes(), &NonFungibleTokenData{
		NftType:             existingNFTTypeUnitID,
		Name:                "ALPHA",
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingLockedNFTUnitID, templates.AlwaysTrueBytes(), &NonFungibleTokenData{
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
		attr       *UnlockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockToken, nil),
			attr:       &UnlockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockToken, existingTokenTypeUnitID),
			attr:       &UnlockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockToken, NewNonFungibleTokenID(nil, []byte{42})),
			attr:       &UnlockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewNonFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already unlocked",
			tx: createTx(t, existingNFTUnitID, &UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "token is already unlocked",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingLockedNFTUnitID, &UnlockTokenAttributes{
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingLockedNFTUnitID, &UnlockTokenAttributes{
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeUnlockToken),
			wantErrStr: `invalid unlock token tx: token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}

	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := m.handleUnlockTokenTx()(tt.tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}
