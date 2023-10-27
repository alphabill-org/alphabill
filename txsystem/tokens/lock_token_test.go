package tokens

import (
	gocrypto "crypto"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	test "github.com/alphabill-org/alphabill/validator/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"
)

var (
	existingNFTTypeUnitID   = NewNonFungibleTokenTypeID(nil, []byte{1})
	existingNFTUnitID       = NewNonFungibleTokenID(nil, []byte{0x02})
	existingLockedNFTUnitID = NewNonFungibleTokenID(nil, []byte{0x03})
)

func TestLockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	attr := &LockTokenAttributes{
		LockStatus:                   1,
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
	}
	tx := createTransactionOrder(t, attr, PayloadTypeLockToken, existingTokenUnitID)
	var roundNo uint64 = 10
	sm, err := handleLockTokenTx(opts)(tx, attr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingTokenUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data())
	d := u.Data().(*fungibleTokenData)

	// verify lock status, backlink and round number is updated
	// verify value and type id is not updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, existingTokenTypeUnitID, d.tokenType)
	require.Equal(t, uint64(existingTokenValue), d.value)
	require.Equal(t, roundNo, d.t)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, attr.LockStatus, d.locked)
}

func TestLockFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}

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
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingTokenUnitID, &LockTokenAttributes{
				LockStatus:                   0,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingTokenUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingTokenUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "invalid predicate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := handleLockTokenTx(opts)(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestLockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)
	attr := &LockTokenAttributes{
		LockStatus:                   1,
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
	}
	tx := createTransactionOrder(t, attr, PayloadTypeLockToken, existingNFTUnitID)
	var roundNo uint64 = 10
	sm, err := handleLockTokenTx(opts)(tx, attr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &nonFungibleTokenData{}, u.Data())
	d := u.Data().(*nonFungibleTokenData)

	// verify lock status, backlink and round number is updated
	require.Equal(t, templates.AlwaysTrueBytes(), u.Bearer())
	require.Equal(t, roundNo, d.t)
	require.Equal(t, tx.Hash(gocrypto.SHA256), d.backlink)
	require.Equal(t, attr.LockStatus, d.locked)
}

func TestLockNFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}

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
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingNFTUnitID, &LockTokenAttributes{
				LockStatus:                   0,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingNFTUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingNFTUnitID, &LockTokenAttributes{
				LockStatus:                   1,
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeLockToken),
			wantErrStr: "invalid predicate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &LockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := handleLockTokenTx(opts)(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func defaultLockOpts(t *testing.T) *Options {
	o := defaultOptions()
	o.state = initStateForLockTxTests(t)
	return o
}

// initStateForLockTxTests creates state with locked and unlocked fungible and non-fungible tokens
func initStateForLockTxTests(t *testing.T) *state.State {
	s := state.NewEmptyState()

	err := s.Apply(state.AddUnit(existingTokenTypeUnitID, templates.AlwaysTrueBytes(), &fungibleTokenTypeData{
		symbol:                   "ALPHA",
		name:                     "A long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		parentTypeId:             nil,
		decimalPlaces:            5,
		subTypeCreationPredicate: templates.AlwaysTrueBytes(),
		tokenCreationPredicate:   templates.AlwaysTrueBytes(),
		invariantPredicate:       templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingTokenUnitID, templates.AlwaysTrueBytes(), &fungibleTokenData{
		tokenType: existingTokenTypeUnitID,
		value:     existingTokenValue,
		t:         0,
		backlink:  make([]byte, 32),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedTokenUnitID, templates.AlwaysTrueBytes(), &fungibleTokenData{
		tokenType: existingTokenTypeUnitID,
		value:     existingTokenValue,
		t:         0,
		backlink:  make([]byte, 32),
		locked:    1,
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTTypeUnitID, templates.AlwaysTrueBytes(), &nonFungibleTokenTypeData{
		symbol:                   "ALPHA",
		name:                     "A long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		subTypeCreationPredicate: templates.AlwaysTrueBytes(),
		tokenCreationPredicate:   templates.AlwaysTrueBytes(),
		invariantPredicate:       templates.AlwaysTrueBytes(),
		dataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingNFTUnitID, templates.AlwaysTrueBytes(), &nonFungibleTokenData{
		nftType:             existingNFTTypeUnitID,
		name:                "ALPHA",
		backlink:            make([]byte, 32),
		dataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)

	err = s.Apply(state.AddUnit(existingLockedNFTUnitID, templates.AlwaysTrueBytes(), &nonFungibleTokenData{
		nftType:             existingNFTTypeUnitID,
		name:                "ALPHA",
		backlink:            make([]byte, 32),
		dataUpdatePredicate: templates.AlwaysTrueBytes(),
		locked:              1,
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
