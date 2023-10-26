package tokens

import (
	gocrypto "crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
)

func TestUnlockFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)

	// create unlock tx
	unlockAttr := &UnlockTokenAttributes{
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
	}
	unlockTx := createTransactionOrder(t, unlockAttr, PayloadTypeUnlockToken, existingLockedTokenUnitID)
	roundNo := uint64(11)
	sm, err := handleUnlockTokenTx(opts)(unlockTx, unlockAttr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedTokenUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &fungibleTokenData{}, u.Data())
	unitData := u.Data().(*fungibleTokenData)

	// verify token is unlocked, backlink and round number is updated
	require.Equal(t, roundNo, unitData.t)
	require.Equal(t, unlockTx.Hash(gocrypto.SHA256), unitData.backlink)
	require.EqualValues(t, 0, unitData.locked)
}

func TestUnlockFT_NotOk(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultLockOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}

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
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "token is already unlocked",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingLockedTokenUnitID, &UnlockTokenAttributes{
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingLockedTokenUnitID, &UnlockTokenAttributes{
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeUnlockToken),
			wantErrStr: "invalid predicate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := handleUnlockTokenTx(opts)(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}

func TestUnlockNFT_Ok(t *testing.T) {
	opts := defaultLockOpts(t)

	// create unlock tx
	unlockAttr := &UnlockTokenAttributes{
		Backlink:                     make([]byte, 32),
		InvariantPredicateSignatures: [][]byte{templates.AlwaysTrueArgBytes()},
	}
	unlockTx := createTransactionOrder(t, unlockAttr, PayloadTypeUnlockToken, existingLockedNFTUnitID)
	roundNo := uint64(11)
	sm, err := handleUnlockTokenTx(opts)(unlockTx, unlockAttr, roundNo)
	require.NoError(t, err)
	require.NotNil(t, sm)
	u, err := opts.state.GetUnit(existingLockedNFTUnitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &nonFungibleTokenData{}, u.Data())
	nftUnitData := u.Data().(*nonFungibleTokenData)

	// verify token is unlocked, backlink and round number is updated
	require.Equal(t, roundNo, nftUnitData.t)
	require.Equal(t, unlockTx.Hash(gocrypto.SHA256), nftUnitData.backlink)
	require.EqualValues(t, 0, nftUnitData.locked)
}

func TestUnlockNFT_NotOk(t *testing.T) {
	// create state with existing NFT tokens
	_, verifier := testsig.CreateSignerAndVerifier(t)
	opts := defaultOpts(t)
	opts.trustBase = map[string]abcrypto.Verifier{"test": verifier}
	err := opts.state.Apply(state.AddUnit(existingNFTTypeUnitID, templates.AlwaysTrueBytes(), &nonFungibleTokenTypeData{
		symbol:                   "ALPHA",
		name:                     "A long name for ALPHA",
		icon:                     &Icon{Type: validIconType, Data: test.RandomBytes(10)},
		subTypeCreationPredicate: templates.AlwaysTrueBytes(),
		tokenCreationPredicate:   templates.AlwaysTrueBytes(),
		invariantPredicate:       templates.AlwaysTrueBytes(),
		dataUpdatePredicate:      templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingNFTUnitID, templates.AlwaysTrueBytes(), &nonFungibleTokenData{
		nftType:             existingNFTTypeUnitID,
		name:                "ALPHA",
		backlink:            make([]byte, 32),
		dataUpdatePredicate: templates.AlwaysTrueBytes(),
	}))
	require.NoError(t, err)
	err = opts.state.Apply(state.AddUnit(existingLockedNFTUnitID, templates.AlwaysTrueBytes(), &nonFungibleTokenData{
		nftType:             existingNFTTypeUnitID,
		name:                "ALPHA",
		backlink:            make([]byte, 32),
		dataUpdatePredicate: templates.AlwaysTrueBytes(),
		locked:              1,
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
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "token is already unlocked",
		},
		{
			name: "invalid backlink",
			tx: createTx(t, existingLockedNFTUnitID, &UnlockTokenAttributes{
				Backlink:                     test.RandomBytes(32),
				InvariantPredicateSignatures: [][]byte{},
			}, PayloadTypeUnlockToken),
			wantErrStr: "the transaction backlink is not equal to the token backlink",
		},
		{
			name: "invalid token invariant predicate argument",
			tx: createTx(t, existingLockedNFTUnitID, &UnlockTokenAttributes{
				Backlink:                     make([]byte, 32),
				InvariantPredicateSignatures: [][]byte{templates.AlwaysFalseBytes()},
			}, PayloadTypeUnlockToken),
			wantErrStr: "invalid predicate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := &UnlockTokenAttributes{}
			require.NoError(t, tt.tx.UnmarshalAttributes(attr))

			sm, err := handleUnlockTokenTx(opts)(tt.tx, attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, sm)
		})
	}
}
