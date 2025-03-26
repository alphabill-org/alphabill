package tokens

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	tokensid "github.com/alphabill-org/alphabill-go-base/testutils/tokens"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func TestModule_validateNopTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)
	counter := uint64(1)

	t.Run("ok with FT unit data", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenTypeID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.FungibleTokenData{Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateNopTx(tx, attr, authProof, exeCtx))
	})

	t.Run("ok with NFT unit data", func(t *testing.T) {
		unitID := tokensid.NewNonFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.NonFungibleTokenData{Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateNopTx(tx, attr, authProof, exeCtx))
	})

	t.Run("ok with FCR unit data", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: counter, OwnerPredicate: templates.AlwaysTrueBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateNopTx(tx, attr, authProof, exeCtx))
	})

	t.Run("ok cannot target FT type", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenTypeID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.FungibleTokenTypeData{Name: "X"}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "nop transaction cannot target token types")
	})

	t.Run("ok cannot target NFT type", func(t *testing.T) {
		unitID := tokensid.NewNonFungibleTokenTypeID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.NonFungibleTokenTypeData{Name: "X"}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "nop transaction cannot target token types")
	})

	t.Run("ok with dummy unit data", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withDummyUnit(unitID))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)
		exeCtx := testctx.NewMockExecutionContext()
		require.NoError(t, module.validateNopTx(tx, attr, authProof, exeCtx))
	})

	t.Run("nok with invalid counter for FT unit", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.FungibleTokenData{Counter: 2}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter for FT data")
	})

	t.Run("nok with invalid counter for NFT unit", func(t *testing.T) {
		unitID := tokensid.NewNonFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.NonFungibleTokenData{Counter: 2}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter for NFT data")
	})

	t.Run("nok with invalid counter for FCR unit", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: 2}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter for FCR")
	})

	t.Run("nok with invalid counter for dummy unit data", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withDummyUnit(unitID))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "nop transaction targeting dummy unit cannot contain counter value")
	})

	t.Run("nok with nil counter for FT unit", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil) // counter is nil
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.FungibleTokenData{Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter for FT data")
	})

	t.Run("nok with nil counter for NFT unit", func(t *testing.T) {
		unitID := tokensid.NewNonFungibleTokenID(t)
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil) // counter is nil
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.NonFungibleTokenData{Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter for NFT data")
	})

	t.Run("nok with nil counter for FCR unit", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil) // counter is nil
		module := newNopModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: 0}))
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "the transaction counter is not equal to the unit counter for FCR")
	})

	t.Run("nok invalid owner for FT unit data", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenTypeID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.FungibleTokenData{Counter: counter, OwnerPredicate: templates.AlwaysFalseBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "evaluating owner predicate")
	})

	t.Run("nok invalid owner for NFT unit data", func(t *testing.T) {
		unitID := tokensid.NewNonFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.NonFungibleTokenData{Counter: counter, OwnerPredicate: templates.AlwaysFalseBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "evaluating owner predicate")
	})

	t.Run("nok invalid owner for FCR unit data", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: counter, OwnerPredicate: templates.AlwaysFalseBytes()}))
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)
		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "evaluating owner predicate")
	})

	t.Run("nok with invalid owner for dummy unit data", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withDummyUnit(unitID))
		tx := createTx(unitID, fcrID, nop.TransactionTypeNOP)
		attr := &nop.Attributes{}
		require.NoError(t, tx.SetAttributes(attr))
		authProof := &nop.AuthProof{OwnerProof: []byte{1}} // tx targeting dummy unit cannot contain owner proof
		require.NoError(t, tx.SetAuthProof(authProof))

		exeCtx := testctx.NewMockExecutionContext()
		require.ErrorContains(t, module.validateNopTx(tx, attr, authProof, exeCtx), "nop transaction targeting dummy unit cannot contain owner proof")
	})

}

func TestModule_executeNopTx(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	t.Run("FT unit ok - counter is incremented", func(t *testing.T) {
		counter := uint64(1)
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.FungibleTokenData{Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify only counter was incremented and all other fields are unchanged
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		ft, ok := u.Data().(*tokens.FungibleTokenData)
		require.True(t, ok)
		require.EqualValues(t, 2, ft.Counter)
	})

	t.Run("NFT unit ok - counter is incremented", func(t *testing.T) {
		counter := uint64(1)
		unitID := tokensid.NewNonFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.NonFungibleTokenData{Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify only counter was incremented and all other fields are unchanged
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		nft, ok := u.Data().(*tokens.NonFungibleTokenData)
		require.True(t, ok)
		require.EqualValues(t, 2, nft.Counter)
	})

	t.Run("FCR unit ok - counter is incremented", func(t *testing.T) {
		counter := uint64(1)
		unitID := tokensid.NewFungibleTokenID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &fcsdk.FeeCreditRecord{Balance: 10, Counter: counter}))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, &counter)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify only counter was incremented and all other fields are unchanged
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		fcr, ok := u.Data().(*fcsdk.FeeCreditRecord)
		require.True(t, ok)
		require.EqualValues(t, 2, fcr.Counter)
		require.EqualValues(t, 10, fcr.Balance)
		require.EqualValues(t, 0, fcr.Locked)
	})

	t.Run("FT type unit ok - nothing is changed", func(t *testing.T) {
		unitID := tokensid.NewFungibleTokenTypeID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.FungibleTokenTypeData{Name: "X"}))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify nothing is changed
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		require.Equal(t, "X", u.Data().(*tokens.FungibleTokenTypeData).Name)
	})

	t.Run("NFT type unit ok - nothing is changed", func(t *testing.T) {
		unitID := tokensid.NewNonFungibleTokenTypeID(t)
		module := newNopModule(t, verifier, withStateUnit(unitID, &tokens.NonFungibleTokenTypeData{Name: "X"}))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify nothing is changed
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		require.NotNil(t, u.Data())
		require.Equal(t, "X", u.Data().(*tokens.NonFungibleTokenTypeData).Name)
	})

	t.Run("Dummy unit ok - nothing is changed", func(t *testing.T) {
		unitID := tokensid.NewNonFungibleTokenTypeID(t)
		module := newNopModule(t, verifier, withDummyUnit(unitID))
		exeCtx := testctx.NewMockExecutionContext()
		tx, attr, authProof := createNopTx(t, unitID, fcrID, nil)

		// execute nop tx
		sm, err := module.executeNopTx(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)

		// verify nothing is changed
		u, err := module.state.GetUnit(unitID, false)
		require.NoError(t, err)
		require.Nil(t, u.Data())
	})
}

func createNopTx(t *testing.T, fromID types.UnitID, fcrID types.UnitID, counter *uint64) (*types.TransactionOrder, *nop.Attributes, *nop.AuthProof) {
	tx := createTx(fromID, fcrID, nop.TransactionTypeNOP)
	attr := &nop.Attributes{
		Counter: counter,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &nop.AuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return tx, attr, authProof
}

func createTx(unitID types.UnitID, fcrID types.UnitID, transactionType uint16) *types.TransactionOrder {
	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:   types.NetworkLocal,
			PartitionID: tokens.DefaultPartitionID,
			UnitID:      unitID,
			Type:        transactionType,
			Attributes:  nil,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		FeeProof: templates.EmptyArgument(),
	}
	return tx
}

type nopModuleOptions func(m *NopModule) error

func newNopModule(t *testing.T, verifier abcrypto.Verifier, opts ...nopModuleOptions) *NopModule {
	options, err := defaultOptions(observability.Default(t))
	require.NoError(t, err)
	options.trustBase = testtb.NewTrustBase(t, verifier)
	options.state = state.NewEmptyState()
	module := NewNopModule(tokensid.PDR(), options)
	for _, opt := range opts {
		require.NoError(t, opt(module))
	}
	return module
}

func withStateUnit(unitID []byte, data types.UnitData) nopModuleOptions {
	return func(m *NopModule) error {
		return m.state.Apply(state.AddUnit(unitID, data))
	}
}

func withDummyUnit(unitID []byte) nopModuleOptions {
	return func(m *NopModule) error {
		return m.state.Apply(state.AddDummyUnit(unitID))
	}
}
