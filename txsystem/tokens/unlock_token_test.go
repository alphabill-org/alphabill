package tokens

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
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

func TestUnlockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))
	// create unlock tx
	unlockAttr := &tokens.UnlockTokenAttributes{Counter: 0}
	unlockTx := createTxOrder(t, existingLockedTokenID, tokens.TransactionTypeUnlockToken, unlockAttr)
	roundNo := uint64(11)
	sm, err := txExecutors.ValidateAndExecute(unlockTx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedTokenID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	unitData := u.Data().(*tokens.FungibleTokenData)

	// verify token is unlocked, counter and round number is updated
	require.Equal(t, uint64(1), unitData.Counter)
	require.EqualValues(t, 0, unitData.Locked)
}

func TestUnlockFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeUnlockToken, nil),
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeUnlockToken, nil),
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000101' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxOrder(t, tokens.NewFungibleTokenID(nil, []byte{42}), tokens.TransactionTypeUnlockToken, nil),
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name:       "token is already unlocked",
			tx:         createTxOrder(t, existingTokenID, tokens.TransactionTypeUnlockToken, &tokens.UnlockTokenAttributes{Counter: 0}),
			wantErrStr: "token is already unlocked",
		},
		{
			name:       "invalid counter",
			tx:         createTxOrder(t, existingLockedTokenID, tokens.TransactionTypeUnlockToken, &tokens.UnlockTokenAttributes{Counter: 1}),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token owner proof",
			tx: createTxOrder(t,
				existingLockedTokenID,
				tokens.TransactionTypeUnlockToken,
				&tokens.UnlockTokenAttributes{Counter: 0},
				testtransaction.WithAuthProof(tokens.UnlockTokenAuthProof{OwnerProof: templates.AlwaysFalseBytes()}),
			),
			wantErrStr: `evaluating owner predicate: executing predicate: "always true" predicate arguments must be empty`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			authProof := &tokens.UnlockTokenAuthProof{}
			require.NoError(t, tt.tx.UnmarshalAuthProof(authProof))

			err := m.validateUnlockTokenTx(tt.tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func TestUnlockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(m.TxHandlers()))
	// create unlock tx
	unlockAttr := &tokens.UnlockTokenAttributes{Counter: 0}
	unlockTx := createTxOrder(t, existingLockedNFTUnitID, tokens.TransactionTypeUnlockToken, unlockAttr)
	roundNo := uint64(11)
	sm, err := txExecutors.ValidateAndExecute(unlockTx, testctx.NewMockExecutionContext(testctx.WithCurrentRound(roundNo)))
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	nftUnitData := u.Data().(*tokens.NonFungibleTokenData)

	// verify token is unlocked, counter and round number is updated
	require.Equal(t, uint64(1), nftUnitData.Counter)
	require.EqualValues(t, 0, nftUnitData.Locked)
}

func TestUnlockNFT_NotOk(t *testing.T) {
	// create state with existing NFT tokens
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = testtb.NewTrustBase(t, verifier)
	err := opts.state.Apply(state.AddUnit(existingNFTTypeUnitID, &tokens.NonFungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenMintingPredicate:    templates.AlwaysTrueBytes(),
		TokenTypeOwnerPredicate:  templates.AlwaysTrueBytes(),
		DataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingNFTUnitID, &tokens.NonFungibleTokenData{
		TypeID:              existingNFTTypeUnitID,
		Name:                "ALPHA",
		OwnerPredicate:      templates.AlwaysTrueBytes(),
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingLockedNFTUnitID, &tokens.NonFungibleTokenData{
		TypeID:              existingNFTTypeUnitID,
		Name:                "ALPHA",
		OwnerPredicate:      templates.AlwaysTrueBytes(),
		Counter:             0,
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
		Locked:              1,
	}))
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTxOrder(t, nil, tokens.TransactionTypeUnlockToken, nil),
			wantErrStr: "not found",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTxOrder(t, existingTokenTypeID, tokens.TransactionTypeUnlockToken, nil),
			wantErrStr: "unit id '000000000000000000000000000000000000000000000000000000000000000101' is not of fungible nor non-fungible token type",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTxOrder(t, tokens.NewNonFungibleTokenID(nil, []byte{42}), tokens.TransactionTypeUnlockToken, nil),
			wantErrStr: fmt.Sprintf("unit %s does not exist", tokens.NewNonFungibleTokenID(nil, []byte{42})),
		},
		{
			name:       "token is already unlocked",
			tx:         createTxOrder(t, existingNFTUnitID, tokens.TransactionTypeUnlockToken, &tokens.UnlockTokenAttributes{Counter: 0}),
			wantErrStr: "token is already unlocked",
		},
		{
			name:       "invalid counter",
			tx:         createTxOrder(t, existingLockedNFTUnitID, tokens.TransactionTypeUnlockToken, &tokens.UnlockTokenAttributes{Counter: 1}),
			wantErrStr: "the transaction counter is not equal to the token counter",
		},
		{
			name: "invalid token owner proof",
			tx: createTxOrder(t,
				existingLockedNFTUnitID,
				tokens.TransactionTypeUnlockToken,
				&tokens.UnlockTokenAttributes{Counter: 0},
				testtransaction.WithAuthProof(tokens.UnlockTokenAuthProof{OwnerProof: templates.AlwaysFalseBytes()}),
			),
			wantErrStr: `evaluating owner predicate: executing predicate: "always true" predicate arguments must be empty`,
		},
	}

	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))
			authProof := &tokens.UnlockTokenAuthProof{}
			require.NoError(t, tt.tx.UnmarshalAuthProof(authProof))

			err := m.validateUnlockTokenTx(tt.tx, attr, authProof, testctx.NewMockExecutionContext(testctx.WithCurrentRound(10)))
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}
