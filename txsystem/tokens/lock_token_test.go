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
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
)

var (
	existingNFTTypeUnitID   = NewNonFungibleTokenTypeID(nil, []byte{1})
	existingNFTUnitID       = NewNonFungibleTokenID(nil, []byte{0x02})
	existingLockedNFTUnitID = NewNonFungibleTokenID(nil, []byte{0x03})
)

func TestLockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	attr := &LockTokenAttributes{
		LockStatus:                   1,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	tx := createTransactionOrder(t, attr, PayloadTypeLockToken, existingTokenUnitID)
	var roundNo uint64 = 10
	sm, err := m.handleLockTokenTx()(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: roundNo})
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingTokenUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &FungibleTokenData{}, u.Data())
	d := u.Data().(*FungibleTokenData)

	// verify lock status, counter and round number is updated
	// verify value and type id is not updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, existingTokenTypeUnitID, d.TokenType)
	require.Equal(t, uint64(existingTokenValue), d.Value)
	require.Equal(t, roundNo, d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, attr.LockStatus, d.Locked)
}

func TestLockFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *LockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockToken, nil),
			attr:       &LockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockToken, existingTokenTypeUnitID),
			attr:       &LockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockToken, NewFungibleTokenID(nil, []byte{42})),
			attr:       &LockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit '%s' does not exist", NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already locked",
			tx: createTx(t, existingLockedTokenUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingTokenUnitID, &LockTokenAttributes{
				LockStatus:                   0,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingTokenUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, PayloadTypeLockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingTokenUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{{8, 4, 0}},
			}, PayloadTypeLockToken),
			wantErrStr: `invalid lock token tx: token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := m.handleLockTokenTx()(tt.tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestLockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	attr := &LockTokenAttributes{
		LockStatus:                   1,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	tx := createTransactionOrder(t, attr, PayloadTypeLockToken, existingNFTUnitID)
	var roundNo uint64 = 10
	sm, err := m.handleLockTokenTx()(tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: roundNo})
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &NonFungibleTokenData{}, u.Data())
	d := u.Data().(*NonFungibleTokenData)

	// verify lock status, counter and round number is updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, roundNo, d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, attr.LockStatus, d.Locked)
}

func TestLockNFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *LockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockToken, nil),
			attr:       &LockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockToken, existingTokenTypeUnitID),
			attr:       &LockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "non-fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockToken, NewNonFungibleTokenID(nil, []byte{42})),
			attr:       &LockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit '%s' does not exist", NewNonFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already locked",
			tx: createTx(t, existingLockedNFTUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingNFTUnitID, &LockTokenAttributes{
				LockStatus:                   0,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingNFTUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, PayloadTypeLockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingNFTUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{{1, 2, 3}},
			}, PayloadTypeLockToken),
			wantErrStr: `invalid lock token tx: token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := m.handleLockTokenTx()(tt.tx, attr, &txsystem.TxExecutionContext{CurrentBlockNr: 10})
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func defaultLockOpts(t *testing.T) *Options {
	o, err := defaultOptions()
	require.NoError(t, err)
	o.state = initStateForLockTxTests(t)
	return o
}

// initStateForLockTxTests creates state with locked and unlocked fungible and non-fungible tokens
func initStateForLockTxTests(t *testing.T) *state.State {
	s := state.NewEmptyState()

	err := s.Apply(state.AddUnit(existingTokenTypeUnitID, templates.AlwaysTrueBytes(), &FungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		ParentTypeId:             nil,
		DecimalPlaces:            5,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingTokenUnitID, templates.AlwaysTrueBytes(), &FungibleTokenData{
		TokenType: existingTokenTypeUnitID,
		Value:     existingTokenValue,
		T:         0,
		Counter:   0,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedTokenUnitID, templates.AlwaysTrueBytes(), &FungibleTokenData{
		TokenType: existingTokenTypeUnitID,
		Value:     existingTokenValue,
		T:         0,
		Counter:   0,
		Locked:    1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTTypeUnitID, templates.AlwaysTrueBytes(), &NonFungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
		DataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTUnitID, templates.AlwaysTrueBytes(), &NonFungibleTokenData{
		NftType:             existingNFTTypeUnitID,
		Name:                "ALPHA",
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedNFTUnitID, templates.AlwaysTrueBytes(), &NonFungibleTokenData{
		NftType:             existingNFTTypeUnitID,
		Name:                "ALPHA",
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
		Locked:              1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(feeCreditID, templates.AlwaysTrueBytes(), &unit.FeeCreditRecord{
		Balance:  100,
		Backlink: make([]byte, 32),
		Timeout:  100,
	}))
	require.NoError(t, err)

	return s
}
