package tokens

import (
	gocrypto "crypto"
	"fmt"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
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
	attr := &tokens.LockTokenAttributes{
		LockStatus:                   1,
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	tx := createTransactionOrder(t, attr, tokens.PayloadTypeLockToken, existingTokenUnitID)
	var roundNo uint64 = 10
	sm, err := m.handleLockTokenTx()(tx, attr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingTokenUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.FungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.FungibleTokenData)

	// verify lock status, backlink and round number is updated
	// verify value and type id is not updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, existingTokenTypeUnitID, d.TokenType)
	require.Equal(t, uint64(existingTokenValue), d.Value)
	require.Equal(t, roundNo, d.T)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.Backlink)
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
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, existingTokenTypeUnitID),
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
			tx: createTx(t, existingLockedTokenUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingTokenUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   0,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingTokenUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingTokenUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{{8, 4, 0}},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: `invalid lock token tx: token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := m.handleLockTokenTx()(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestLockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	m, err := NewLockTokensModule(opts)
	require.NoError(t, err)
	attr := &tokens.LockTokenAttributes{
		LockStatus:                   1,
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
	}
	tx := createTransactionOrder(t, attr, tokens.PayloadTypeLockToken, existingNFTUnitID)
	var roundNo uint64 = 10
	sm, err := m.handleLockTokenTx()(tx, attr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &tokens.NonFungibleTokenData{}, u.Data())
	d := u.Data().(*tokens.NonFungibleTokenData)

	// verify lock status, backlink and round number is updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, roundNo, d.T)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.Backlink)
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
			tx:         createTransactionOrder(t, nil, tokens.PayloadTypeLockToken, existingTokenTypeUnitID),
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
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingNFTUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   0,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingNFTUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.EmptyArgument()},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingNFTUnitID, &tokens.LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{{1, 2, 3}},
			}, tokens.PayloadTypeLockToken),
			wantErrStr: `invalid lock token tx: token type InvariantPredicate: executing predicate [0] in the chain: executing predicate: "always true" predicate arguments must be empty`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &tokens.LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := m.handleLockTokenTx()(tt.tx, attr, 10)
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

	err := s.Apply(state.AddUnit(existingTokenTypeUnitID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenTypeData{
		Symbol:                   "ALPHA",
		Name:                     "A long name for ALPHA",
		Icon:                     &tokens.Icon{Type: validIconType, Data: test.RandomBytes(10)},
		ParentTypeId:             nil,
		DecimalPlaces:            5,
		SubTypeCreationPredicate: templates.AlwaysTrueBytes(),
		TokenCreationPredicate:   templates.AlwaysTrueBytes(),
		InvariantPredicate:       templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingTokenUnitID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenData{
		TokenType: existingTokenTypeUnitID,
		Value:     existingTokenValue,
		T:         0,
		Backlink:  make([]byte, 32),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedTokenUnitID, templates.AlwaysTrueBytes(), &tokens.FungibleTokenData{
		TokenType: existingTokenTypeUnitID,
		Value:     existingTokenValue,
		T:         0,
		Backlink:  make([]byte, 32),
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
		NftType:             existingNFTTypeUnitID,
		Name:                "ALPHA",
		Backlink:            make([]byte, 32),
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedNFTUnitID, templates.AlwaysTrueBytes(), &tokens.NonFungibleTokenData{
		NftType:             existingNFTTypeUnitID,
		Name:                "ALPHA",
		Backlink:            make([]byte, 32),
		DataUpdatePredicate: templates.AlwaysTrueBytes(),
		Locked:              1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(feeCreditID, templates.AlwaysTrueBytes(), &fc.FeeCreditRecord{
		Balance:  100,
		Backlink: make([]byte, 32),
		Timeout:  100,
	}))
	require.NoError(t, err)

	return s
}
