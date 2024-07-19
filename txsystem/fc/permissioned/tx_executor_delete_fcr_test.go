package permissioned

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func TestValidateDeleteFCR(t *testing.T) {
	// generate keys
	adminKeySigner, adminKeyVerifier := testsig.CreateSignerAndVerifier(t)
	adminPubKey, err := adminKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	_, userKeyVerifier := testsig.CreateSignerAndVerifier(t)
	userPubKey, err := userKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	// create fee credit module
	stateTree := state.NewEmptyState()
	systemID := types.SystemID(5)
	fcrUnitType := []byte{1}
	adminOwnerCondition := templates.NewP2pkh256BytesFromKey(adminPubKey)
	m, err := NewFeeCreditModule(
		WithSystemIdentifier(systemID),
		WithState(stateTree),
		WithFeeCreditRecordUnitType(fcrUnitType),
		WithAdminOwnerCondition(adminOwnerCondition),
	)
	require.NoError(t, err)

	// common default values used in each test
	fcrOwnerCondition := templates.NewP2pkh256BytesFromKey(userPubKey)
	timeout := uint64(10)
	fcrID := newFeeCreditRecordID(fcrOwnerCondition, fcrUnitType, timeout)

	t.Run("ok", func(t *testing.T) {
		tx, attr, err := newDeleteFeeTx(adminKeySigner, systemID, fcrID, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateDeleteFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.NoError(t, err)
	})

	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx, attr, err := newDeleteFeeTx(adminKeySigner, systemID, fcrID, timeout, []byte{1}, nil)
		require.NoError(t, err)
		err = m.validateDeleteFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "fee tx cannot contain fee credit reference")
	})

	t.Run("FeeProof is not nil", func(t *testing.T) {
		tx, attr, err := newDeleteFeeTx(adminKeySigner, systemID, fcrID, timeout, nil, []byte{1})
		require.NoError(t, err)
		err = m.validateDeleteFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "fee tx cannot contain fee authorization proof")
	})

	t.Run("Invalid unit type byte", func(t *testing.T) {
		// create new fcrID with invalid type byte
		fcrUnitType := []byte{2}
		fcrID := newFeeCreditRecordID(fcrOwnerCondition, fcrUnitType, timeout)
		tx, attr, err := newDeleteFeeTx(adminKeySigner, systemID, fcrID, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateDeleteFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "invalid unit type for unitID")
	})

	t.Run("Fee credit record does not exists", func(t *testing.T) {
		tx, attr, err := newDeleteFeeTx(adminKeySigner, systemID, fcrID, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateDeleteFCR(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithErr(avl.ErrNotFound)))
		require.ErrorContains(t, err, "get fcr unit error: not found")
	})

	t.Run("Invalid signature", func(t *testing.T) {
		// sign tx with random non-admin key
		signer, _ := testsig.CreateSignerAndVerifier(t)
		tx, attr, err := newDeleteFeeTx(signer, systemID, fcrID, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateDeleteFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "invalid owner proof")
	})
}

func TestExecuteDeleteFCR(t *testing.T) {
	// generate keys
	adminKeySigner, adminKeyVerifier := testsig.CreateSignerAndVerifier(t)
	adminPubKey, err := adminKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	_, userKeyVerifier := testsig.CreateSignerAndVerifier(t)
	userPubKey, err := userKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	// create fee credit module
	stateTree := state.NewEmptyState()
	systemID := types.SystemID(5)
	fcrUnitType := []byte{1}
	adminOwnerCondition := templates.NewP2pkh256BytesFromKey(adminPubKey)
	m, err := NewFeeCreditModule(
		WithSystemIdentifier(systemID),
		WithState(stateTree),
		WithFeeCreditRecordUnitType(fcrUnitType),
		WithAdminOwnerCondition(adminOwnerCondition),
	)
	require.NoError(t, err)

	// add unit to state tree
	fcrOwnerCondition := templates.NewP2pkh256BytesFromKey(userPubKey)
	timeout := uint64(10)
	fcrID := newFeeCreditRecordID(fcrOwnerCondition, fcrUnitType, timeout)
	err = stateTree.Apply(state.AddUnit(fcrID, fcrOwnerCondition, &fc.FeeCreditRecord{}))
	require.NoError(t, err)

	// create tx
	tx, attr, err := newDeleteFeeTx(adminKeySigner, systemID, fcrID, timeout, nil, nil)
	require.NoError(t, err)

	// execute tx
	sm, err := m.executeDeleteFCR(tx, attr, testctx.NewMockExecutionContext(t))
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify server metadata
	require.EqualValues(t, 0, sm.ActualFee)
	require.Len(t, sm.TargetUnits, 1)
	require.Equal(t, fcrID, sm.TargetUnits[0])
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

	// verify state was updated (unit still exists but owner predicate is set to AlwaysFalse)
	unit, err := stateTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	require.NotNil(t, unit)
	require.Equal(t, templates.AlwaysFalseBytes(), unit.Bearer())
}

func newDeleteFeeTx(adminKey crypto.Signer, systemID types.SystemID, unitID []byte, timeout uint64, fcrID, feeProof []byte) (*types.TransactionOrder, *permissioned.DeleteFeeCreditAttributes, error) {
	attr := &permissioned.DeleteFeeCreditAttributes{}
	payload, err := newTxPayload(systemID, permissioned.PayloadTypeDeleteFCR, unitID, fcrID, timeout, nil, attr)
	if err != nil {
		return nil, nil, err
	}
	payloadSig, err := signPayload(payload, adminKey)
	if err != nil {
		return nil, nil, err
	}
	adminKeyVerifier, err := adminKey.Verifier()
	if err != nil {
		return nil, nil, err
	}
	adminPublicKey, err := adminKeyVerifier.MarshalPublicKey()
	if err != nil {
		return nil, nil, err
	}
	txo := &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: templates.NewP2pkh256SignatureBytes(payloadSig, adminPublicKey),
		FeeProof:   feeProof,
	}
	return txo, attr, nil
}
