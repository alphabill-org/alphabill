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
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func TestValidateCreateFCR(t *testing.T) {
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
	m, err := NewFeeCreditModule(systemID, stateTree, fcrUnitType, adminOwnerCondition)
	require.NoError(t, err)

	// common default values used in each test
	fcrOwnerCondition := templates.NewP2pkh256BytesFromKey(userPubKey)
	timeout := uint64(10)
	fcrID := newFeeCreditRecordID(fcrOwnerCondition, fcrUnitType, timeout)

	t.Run("ok", func(t *testing.T) {
		tx, attr, err := newCreateFeeTx(adminKeySigner, systemID, fcrID, fcrOwnerCondition, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateCreateFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.NoError(t, err)
	})

	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx, attr, err := newCreateFeeTx(adminKeySigner, systemID, fcrID, fcrOwnerCondition, timeout, []byte{1}, nil)
		require.NoError(t, err)
		err = m.validateCreateFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "fee tx cannot contain fee credit reference")
	})

	t.Run("FeeProof is not nil", func(t *testing.T) {
		tx, attr, err := newCreateFeeTx(adminKeySigner, systemID, fcrID, fcrOwnerCondition, timeout, nil, []byte{1})
		require.NoError(t, err)
		err = m.validateCreateFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "fee tx cannot contain fee authorization proof")
	})

	t.Run("Invalid unit type byte", func(t *testing.T) {
		// create new fcrID with invalid type byte
		fcrUnitType := []byte{2}
		fcrID := newFeeCreditRecordID(fcrOwnerCondition, fcrUnitType, timeout)
		tx, attr, err := newCreateFeeTx(adminKeySigner, systemID, fcrID, fcrOwnerCondition, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateCreateFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "invalid unit type for unitID")
	})

	t.Run("Invalid fee credit record ID", func(t *testing.T) {
		// change timeout to 11 causing FCR to be incorrectly calculated
		timeout := uint64(11)
		tx, attr, err := newCreateFeeTx(adminKeySigner, systemID, fcrID, fcrOwnerCondition, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateCreateFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "tx.unitID is not equal to expected fee credit record id")
	})

	t.Run("Fee credit record already exists", func(t *testing.T) {
		tx, attr, err := newCreateFeeTx(adminKeySigner, systemID, fcrID, fcrOwnerCondition, timeout, nil, nil)
		require.NoError(t, err)
		unit := state.NewUnit(fcrOwnerCondition, &fc.FeeCreditRecord{Balance: 1e8, Timeout: timeout})
		err = m.validateCreateFCR(tx, attr, testctx.NewMockExecutionContext(t, testctx.WithUnit(unit)))
		require.ErrorContains(t, err, "fee credit record already exists")
	})

	t.Run("Invalid signature", func(t *testing.T) {
		// sign tx with random non-admin key
		signer, _ := testsig.CreateSignerAndVerifier(t)
		tx, attr, err := newCreateFeeTx(signer, systemID, fcrID, fcrOwnerCondition, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateCreateFCR(tx, attr, testctx.NewMockExecutionContext(t))
		require.ErrorContains(t, err, "invalid owner proof")
	})
}

func TestExecuteCreateFCR(t *testing.T) {
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
	m, err := NewFeeCreditModule(systemID, stateTree, fcrUnitType, adminOwnerCondition)
	require.NoError(t, err)

	// create tx
	fcrOwnerCondition := templates.NewP2pkh256BytesFromKey(userPubKey)
	timeout := uint64(10)
	fcrID := newFeeCreditRecordID(fcrOwnerCondition, fcrUnitType, timeout)

	tx, attr, err := newCreateFeeTx(adminKeySigner, systemID, fcrID, fcrOwnerCondition, timeout, nil, nil)
	require.NoError(t, err)

	// execute tx
	sm, err := m.executeCreateFCR(tx, attr, testctx.NewMockExecutionContext(t))
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify server metadata
	require.EqualValues(t, 0, sm.ActualFee)
	require.Len(t, sm.TargetUnits, 1)
	require.Equal(t, fcrID, sm.TargetUnits[0])
	require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)

	// verify state was updated
	unit, err := stateTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	require.NotNil(t, unit)
	unitData := unit.Data()
	require.NotNil(t, unitData)
	fcr, ok := unitData.(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 1e8, fcr.Balance)
	require.EqualValues(t, 0, fcr.Counter)
	require.EqualValues(t, 10, fcr.Timeout)
	require.EqualValues(t, 0, fcr.Locked)
}

func newFeeCreditRecordID(ownerPredicate []byte, fcrUnitType []byte, timeout uint64) types.UnitID {
	unitPart := fc.NewFeeCreditRecordUnitPart(ownerPredicate, timeout)
	return types.NewUnitID(33, nil, unitPart, fcrUnitType)
}

func newCreateFeeTx(adminKey crypto.Signer, systemID types.SystemID, unitID, fcrOwnerCondition []byte, timeout uint64, fcrID, feeProof []byte) (*types.TransactionOrder, *permissioned.CreateFeeCreditAttributes, error) {
	attr := &permissioned.CreateFeeCreditAttributes{
		FeeCreditOwnerCondition: fcrOwnerCondition,
	}
	payload, err := newTxPayload(systemID, permissioned.PayloadTypeCreateFCR, unitID, fcrID, timeout, nil, attr)
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

func newTxPayload(systemID types.SystemID, txType string, unitID, fcrID types.UnitID, timeout uint64, refNo []byte, attr interface{}) (*types.Payload, error) {
	attrBytes, err := types.Cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	return &types.Payload{
		SystemID:   systemID,
		Type:       txType,
		UnitID:     unitID,
		Attributes: attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: 10,
			FeeCreditRecordID: fcrID,
			ReferenceNumber:   refNo,
		},
	}, nil
}

func signPayload(payload *types.Payload, signer crypto.Signer) ([]byte, error) {
	payloadBytes, err := payload.Bytes()
	if err != nil {
		return nil, err
	}
	payloadSig, err := signer.SignBytes(payloadBytes)
	if err != nil {
		return nil, err
	}
	return payloadSig, nil
}
