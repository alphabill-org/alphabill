package tokens

import (
	gocrypto "crypto"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
)

var (
	existingNFTTypeUnitID   = NewNonFungibleTokenTypeID(nil, []byte{1})
	existingNFTUnitID       = NewNonFungibleTokenID(nil, []byte{0x02})
	existingLockedNFTUnitID = NewNonFungibleTokenID(nil, []byte{0x03})
)

func TestLockAndUnlockNFT_Ok(t *testing.T) {
	opts := defaultOptions()
	opts.state = state.NewEmptyState()
	err := opts.state.Apply(state.AddUnit(existingNFTTypeUnitID, script.PredicateAlwaysTrue(), &nonFungibleTokenTypeData{
		symbol:                   "ALPHA",
		name:                     "A long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
		dataUpdatePredicate:      script.PredicateAlwaysTrue(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingNFTUnitID, script.PredicateAlwaysTrue(), &nonFungibleTokenData{
		nftType:             existingNFTTypeUnitID,
		name:                "ALPHA",
		backlink:            make([]byte, 32),
		dataUpdatePredicate: script.PredicateAlwaysTrue(),
	}))
	require.NoError(t, err)

	// create lock nft tx
	attr := &LockNonFungibleTokenAttributes{
		LockStatus:                   1,
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	lockTx := createTransactionOrder(t, attr, PayloadTypeLockNFT, existingNFTUnitID)
	roundNo := uint64(10)
	sm, err := handleLockNonFungibleTokenTx(opts)(lockTx, attr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &nonFungibleTokenData{}, u.Data())
	nftUnitData := u.Data().(*nonFungibleTokenData)

	// verify lock status, backlink and round number is updated
	require.Equal(t, roundNo, nftUnitData.t)
	require.Equal(t, lockTx.Hash(gocrypto.SHA256), nftUnitData.backlink)
	require.Equal(t, attr.LockStatus, nftUnitData.locked)

	// create unlock nft tx
	unlockAttr := &UnlockNonFungibleTokenAttributes{
		Backlink:                     lockTx.Hash(gocrypto.SHA256),
		InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
	}
	unlockTx := createTransactionOrder(t, unlockAttr, PayloadTypeUnlockNFT, existingNFTUnitID)
	roundNo = uint64(11)
	sm, err = handleUnlockNonFungibleTokenTx(opts)(unlockTx, unlockAttr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err = opts.state.GetUnit(existingNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &nonFungibleTokenData{}, u.Data())
	nftUnitData = u.Data().(*nonFungibleTokenData)

	// verify token is unlocked, backlink and round number is updated
	require.Equal(t, roundNo, nftUnitData.t)
	require.Equal(t, unlockTx.Hash(gocrypto.SHA256), nftUnitData.backlink)
	require.EqualValues(t, 0, nftUnitData.locked)
}

func TestLockNFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *LockFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockFungibleToken, nil),
			attr:       &LockFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockFungibleToken, existingTokenTypeUnitID),
			attr:       &LockFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeLockFungibleToken, NewFungibleTokenID(nil, []byte{42})),
			attr:       &LockFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already locked",
			tx: createTx(t, existingLockedTokenUnitID, &LockFungibleTokenAttributes{
				LockStatus:                   1,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}, PayloadTypeLockFungibleToken),
			wantErrStr: "token is already locked",
		},
		{
			name: "lock status zero",
			tx: createTx(t, existingTokenUnitID, &LockFungibleTokenAttributes{
				LockStatus:                   0,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}, PayloadTypeLockFungibleToken),
			wantErrStr: "lock status cannot be zero-value",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingTokenUnitID, &LockFungibleTokenAttributes{
				LockStatus:                   1,
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}, PayloadTypeLockFungibleToken),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingTokenUnitID, &LockFungibleTokenAttributes{
				LockStatus:                   1,
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}, PayloadTypeLockFungibleToken),
			wantErrStr: "script execution result yielded non-clean stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &LockFungibleTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := handleLockFungibleTokenTx(opts)(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestUnlockNFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}
	err := opts.state.Apply(state.AddUnit(existingNFTTypeUnitID, script.PredicateAlwaysTrue(), &nonFungibleTokenTypeData{
		symbol:                   "ALPHA",
		name:                     "A long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		subTypeCreationPredicate: script.PredicateAlwaysTrue(),
		tokenCreationPredicate:   script.PredicateAlwaysTrue(),
		invariantPredicate:       script.PredicateAlwaysTrue(),
		dataUpdatePredicate:      script.PredicateAlwaysTrue(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingNFTUnitID, script.PredicateAlwaysTrue(), &nonFungibleTokenData{
		nftType:             existingNFTTypeUnitID,
		name:                "ALPHA",
		backlink:            make([]byte, 32),
		dataUpdatePredicate: script.PredicateAlwaysTrue(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingLockedNFTUnitID, script.PredicateAlwaysTrue(), &nonFungibleTokenData{
		nftType:             existingNFTTypeUnitID,
		name:                "ALPHA",
		backlink:            make([]byte, 32),
		dataUpdatePredicate: script.PredicateAlwaysTrue(),
		locked:              1,
	}))
	require.NoError(t, err)

	tests := []struct {
		name       string
		tx         *types.TransactionOrder
		attr       *UnlockNonFungibleTokenAttributes
		wantErrStr string
	}{
		{
			name:       "unit ID is nil",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockNFT, nil),
			attr:       &UnlockNonFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "unit ID has wrong type",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockNFT, existingTokenTypeUnitID),
			attr:       &UnlockNonFungibleTokenAttributes{},
			wantErrStr: "invalid unit ID",
		},
		{
			name:       "fungible token does not exists",
			tx:         createTransactionOrder(t, nil, PayloadTypeUnlockNFT, NewNonFungibleTokenID(nil, []byte{42})),
			attr:       &UnlockNonFungibleTokenAttributes{},
			wantErrStr: fmt.Sprintf("unit %s does not exist", NewNonFungibleTokenID(nil, []byte{42})),
		},
		{
			name: "token is already unlocked",
			tx: createTx(t, existingNFTUnitID, &UnlockNonFungibleTokenAttributes{
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}, PayloadTypeUnlockNFT),
			wantErrStr: "token is already unlocked",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingLockedNFTUnitID, &UnlockNonFungibleTokenAttributes{
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{script.PredicateArgumentEmpty()},
			}, PayloadTypeUnlockNFT),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingLockedNFTUnitID, &UnlockNonFungibleTokenAttributes{
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{script.PredicateAlwaysFalse()},
			}, PayloadTypeUnlockNFT),
			wantErrStr: "script execution result yielded non-clean stack",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &UnlockNonFungibleTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := handleUnlockNonFungibleTokenTx(opts)(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}
