package tokens

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	tokenid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/stretchr/testify/require"
)

var (
	existingNFTTypeUnitID   types.UnitID = append(make(types.UnitID, 31), 1, tokens.NonFungibleTokenTypeUnitType)
	existingNFTUnitID       types.UnitID = append(make(types.UnitID, 31), 2, tokens.NonFungibleTokenUnitType)
	existingLockedNFTUnitID types.UnitID = append(make(types.UnitID, 31), 3, tokens.NonFungibleTokenUnitType)
)

func TestLockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))
	attr := &tokens.LockTokenAttributes{
		LockStatus: 1,
		Counter:    0,
	}
	tx := createTxOrder(t, existingTokenID, tokens.TransactionTypeLockToken, attr,
		testtransaction.WithAuthProof(tokens.LockTokenAuthProof{OwnerProof: templates.EmptyArgument()}),
	)
	var roundNo uint64 = 10
	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingTokenID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	// verify lock status, counter and round number is updated
	// verify value and type id is not updated
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.Owner())
	require.Equal(t, existingTokenTypeID, d.TypeID)
	require.Equal(t, uint64(existingTokenValue), d.Value)
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, attr.LockStatus, d.Locked)
}

func TestLockFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	m, err := NewLockTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	tfUnitID := tokenid.NewFungibleTokenID(t)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeLockToken, nil),
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeLockToken, nil),
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000101' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxOrder(t, tfUnitID, tokens.TransactionTypeLockToken, nil),
			wantErrStr: fmt.Sprintf("unit '%s' does not exist", tfUnitID),
		},
		{
			name: "token is already locked",
			tx: createTxOrder(t, existingLockedTokenID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 1,
				Counter:    0,
			}, testtransaction.WithAuthProof([][]byte{templates.EmptyArgument()})),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 0,
				Counter:    0,
			}),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid counter",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 1,
				Counter:    1,
			}),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTxOrder(t, existingTokenID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 1,
				Counter:    0,
			}, testtransaction.WithAuthProof(tokens.LockTokenAuthProof{OwnerProof: []byte{8, 4, 0}})),
			wantErrStr: `evaluating owner predicate: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			authProof := &tokens.LockTokenAuthProof{}
			require.NoError(t, tt.tx.UnmarshalAuthProof(authProof))

			err := m.validateLockTokenTx(tt.tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestLockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))

	attr := &tokens.LockTokenAttributes{
		LockStatus: 1,
		Counter:    0,
	}
	tx := createTxOrder(t, existingNFTUnitID, tokens.TransactionTypeLockToken, attr)
	var roundNo uint64 = 10
	sm, err := txExecutors.ValidateAndExecute(tx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)

	// verify lock status, counter and round number is updated
	require.EqualValues(t, templates.AlwaysTrueBytes(), d.Owner())
	require.Equal(t, uint64(1), d.Counter)
	require.Equal(t, attr.LockStatus, d.Locked)
}

func TestLockNFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	m, err := NewLockTokensModule(tokenid.PDR(), opts)
	require.NoError(t, err)
	nftID := tokenid.NewNonFungibleTokenID(t)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeLockToken, nil),
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeLockToken, nil),
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000101' is not of fungible nor non-fungible token type",
		},
		{
			name:       "non-fungible token does not exists",
			tx:         createTxOrder(t, nftID, tokens.TransactionTypeLockToken, nil),
			wantErrStr: fmt.Sprintf("unit '%s' does not exist", nftID),
		},
		{
			name: "token is already locked",
			tx: createTxOrder(t, existingLockedNFTUnitID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 1,
				Counter:    0,
			}),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTxOrder(t, existingNFTUnitID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 0,
				Counter:    0,
			}),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid counter",
			tx: createTxOrder(t, existingNFTUnitID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 1,
				Counter:    1,
			}),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token owner proof",
			tx: createTxOrder(t, existingNFTUnitID, tokens.TransactionTypeLockToken, &tokens.LockTokenAttributes{
				LockStatus: 1,
				Counter:    0,
			}, testtransaction.WithAuthProof(tokens.LockTokenAuthProof{OwnerProof: []byte{1, 2, 3}})),
			wantErrStr: `evaluating owner predicate: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			authProof := &tokens.LockTokenAuthProof{}
			require.NoError(t, tt.tx.UnmarshalAuthProof(authProof))

			err := m.validateLockTokenTx(tt.tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
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

	err := s.Apply(state.AddUnit(existingTokenTypeID, &tokens.FungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		ParentTypeID:             nil,
		DecimalPlaces:            5,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingTokenID, &tokens.FungibleTokenData{
		TypeID:         existingTokenTypeID,
		Value:          existingTokenValue,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        0,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedTokenID, &tokens.FungibleTokenData{
		TypeID:         existingTokenTypeID,
		Value:          existingTokenValue,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        0,
		Locked:         1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTTypeUnitID, &tokens.NonFungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
		DataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTUnitID, &tokens.NonFungibleTokenData{
		TypeID:              existingNFTTypeUnitID,
		Name:                "ALPHA",
		OwnerPredicate:      templates.AlwaysTrueBytes(),
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedNFTUnitID, &tokens.NonFungibleTokenData{
		TypeID:              existingNFTTypeUnitID,
		Name:                "ALPHA",
		OwnerPredicate:      templates.AlwaysTrueBytes(),
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
		Locked:              1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(feeCreditID, &fc.FeeCreditRecord{
		Balance:        100,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Counter:        10,
		MinLifetime:    100,
	}))
	require.NoError(t, err)

	return s
}
