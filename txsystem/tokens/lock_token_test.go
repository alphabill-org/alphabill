package tokens

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/stretchr/testify/require"
)

var (
	existingNFTTypeUnitID   = tokens.NewNonFungibleTokenTypeID(nil, []byte{1})
	existingNFTUnitID       = tokens.NewNonFungibleTokenID(nil, []byte{0x02})
	existingLockedNFTUnitID = tokens.NewNonFungibleTokenID(nil, []byte{0x03})
)

func TestLockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))
	attr := &tokens.LockTokenAttributes{
		LockStatus:                   1,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	tx := createTransactionOrder(t, attr, tokens.PayloadTypeLockToken, existingTokenID)
	var roundNo uint64 = 10
	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingTokenID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	// verify lock status, counter and round number is updated
	// verify value and type id is not updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, existingTokenTypeID, d.TokenType)
	require.Equal(t, uint64(existingTokenValue), d.Value)
	require.Equal(t, roundNo, d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, attr.LockStatus, d.Locked)
}

func TestLockFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.LockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, nil),
			attr:       &tokens.LockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, existingTokenTypeID),
			attr:       &tokens.LockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, tokens.NewFungibleTokenID(nil, []byte{42})),
			attr:       &tokens.LockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit '%s' does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already locked",
			tx: createTx(t, existingLockedTokenID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingTokenID, &tokens.LockTokenAttributes{
				LockStatus:                   0,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingTokenID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingTokenID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{{8, 4, 0}},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: `executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			err := m.validateLockTokenTx(tt.tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestLockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	attr := &tokens.LockTokenAttributes{
		LockStatus:                   1,
		Counter:                      0,
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	tx := createTransactionOrder(t, attr, tokens.PayloadTypeLockToken, existingNFTUnitID)
	var roundNo uint64 = 10
	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)

	// verify lock status, counter and round number is updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, roundNo, d.T)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, attr.LockStatus, d.Locked)
}

func TestLockNFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *tokens.LockTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, nil),
			attr:       &tokens.LockTokenAttributes{},
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, existingTokenTypeID),
			attr:       &tokens.LockTokenAttributes{},
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000120' is not of fungible nor non-fungible token type",
		},
		{
			name:       "non-fungible token does not exists",
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, tokens.NewNonFungibleTokenID(nil, []byte{42})),
			attr:       &tokens.LockTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit '%s' does not exist", tokens.NewNonFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already locked",
			tx: createTx(t, existingLockedNFTUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingNFTUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   0,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid counter",
			tx: createTx(t, existingNFTUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      1,
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingNFTUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Counter:                      0,
				InvariantPredicateSignatures: [][]byte{{1, 2, 3}},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: `executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			err := m.validateLockTokenTx(tt.tx, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
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

	err := s.Apply(state.AddUnit(existingTokenTypeID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		ParentTypeID:             nil,
		DecimalPlaces:            5,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingTokenID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenData{
		TokenType: existingTokenTypeID,
		Value:     existingTokenValue,
		T:         0,
		Counter:   0,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedTokenID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenData{
		TokenType: existingTokenTypeID,
		Value:     existingTokenValue,
		T:         0,
		Counter:   0,
		Locked:    1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTTypeUnitID, templates.AlwaysTrueBytes(), &tokens.NonFungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
		DataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTUnitID, templates.AlwaysTrueBytes(), &tokens.NonFungibleTokenData{
		TypeID:              existingNFTTypeUnitID,
		Name:                "ALPHA",
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedNFTUnitID, templates.AlwaysTrueBytes(), &tokens.NonFungibleTokenData{
		TypeID:              existingNFTTypeUnitID,
		Name:                "ALPHA",
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
		Locked:              1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(feeCreditID, templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{
		Balance: 100,
		Counter: 10,
		Timeout: 100,
	}))
	require.NoError(t, err)

	return s
}
