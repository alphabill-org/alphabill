package permissioned

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc/permissioned"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	"github.com/stretchr/testify/require"
)

func TestValidateSetFC(t *testing.T) {
	// generate keys
	adminKeySigner, adminKeyVerifier := testsig.CreateSignerAndVerifier(t)
	adminPubKey, err := adminKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	_, userKeyVerifier := testsig.CreateSignerAndVerifier(t)
	userPubKey, err := userKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	// create fee credit module
	stateTree := state.NewEmptyState()
	networkID := types.NetworkID(5)
	partitionID := types.PartitionID(5)
	fcrUnitType := tokens.FeeCreditRecordUnitType
	adminOwnerPredicate := templates.NewP2pkh256BytesFromKey(adminPubKey)
	m, err := NewFeeCreditModule(networkID, partitionID, stateTree, fcrUnitType, adminOwnerPredicate)
	require.NoError(t, err)

	// common default values used in each test
	fcrOwnerPredicate := templates.NewP2pkh256BytesFromKey(userPubKey)
	timeout := uint64(10)
	fcrID := newFeeCreditRecordID(t, fcrOwnerPredicate, fcrUnitType, timeout)

	t.Run("FeeCreditRecordID is not nil", func(t *testing.T) {
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, []byte{1}, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
		require.ErrorContains(t, err, "fee transaction cannot contain fee credit reference")
	})

	t.Run("FeeProof is not nil", func(t *testing.T) {
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, nil, []byte{1})
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
		require.ErrorContains(t, err, "fee transaction cannot contain fee authorization proof")
	})

	t.Run("Invalid unit type byte", func(t *testing.T) {
		// create new fcrID with invalid type byte
		fcrUnitType := []byte{2}
		fcrID := newFeeCreditRecordID(t, fcrOwnerPredicate, fcrUnitType, timeout)
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
		require.ErrorContains(t, err, "invalid unit type for unitID")
	})

	t.Run("Invalid signature", func(t *testing.T) {
		// sign transaction with random non-admin key
		signer, _ := testsig.CreateSignerAndVerifier(t)
		tx, attr, authProof, err := newSetFeeCreditTx(signer, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
		require.ErrorContains(t, err, "invalid owner proof")
	})

	t.Run("Add FCR: ok", func(t *testing.T) {
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
		require.NoError(t, err)
	})

	t.Run("Add FCR: invalid fee credit record ID", func(t *testing.T) {
		// change timeout to 11 causing FCR to be incorrectly calculated
		timeout := uint64(11)
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
		require.ErrorContains(t, err, "tx.unitID is not equal to expected fee credit record id")
	})

	t.Run("Add FCR: Invalid counter", func(t *testing.T) {
		counter := uint64(0)
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, &counter, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
		require.ErrorContains(t, err, "invalid counter: must be nil when creating a new fee credit record")
	})

	t.Run("Update FCR: ok", func(t *testing.T) {
		fcrUnit := state.NewUnit(&fc.FeeCreditRecord{Balance: 1e8, MinLifetime: timeout, OwnerPredicate: fcrOwnerPredicate})
		exeCtx := testctx.NewMockExecutionContext(testctx.WithUnit(fcrUnit))
		counter := uint64(0)
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, &counter, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, exeCtx)
		require.NoError(t, err)
	})

	t.Run("Update FCR: counter is nil", func(t *testing.T) {
		fcrUnit := state.NewUnit(&fc.FeeCreditRecord{Balance: 1e8, MinLifetime: timeout, OwnerPredicate: fcrOwnerPredicate})
		exeCtx := testctx.NewMockExecutionContext(testctx.WithUnit(fcrUnit))
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, exeCtx)
		require.ErrorContains(t, err, "invalid counter: must not be nil when updating an existing fee credit record")
	})

	t.Run("Update FCR: invalid counter", func(t *testing.T) {
		fcrUnit := state.NewUnit(&fc.FeeCreditRecord{Balance: 1e8, MinLifetime: timeout, OwnerPredicate: fcrOwnerPredicate})
		exeCtx := testctx.NewMockExecutionContext(testctx.WithUnit(fcrUnit))
		counter := uint64(1)
		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, &counter, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, exeCtx)
		require.ErrorContains(t, err, "invalid counter: tx.Counter=1 fcr.Counter=0")
	})

	t.Run("Update FCR: invalid target owner predicate", func(t *testing.T) {
		fcrUnit := state.NewUnit(&fc.FeeCreditRecord{Balance: 1e8, MinLifetime: timeout, OwnerPredicate: fcrOwnerPredicate})
		exeCtx := testctx.NewMockExecutionContext(testctx.WithUnit(fcrUnit))
		counter := uint64(0)

		// calculate owner predicate for random key
		_, verifier := testsig.CreateSignerAndVerifier(t)
		randomPubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)
		randomOwnerPredicate := templates.NewP2pkh256BytesFromKey(randomPubKey)

		tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, randomOwnerPredicate, &counter, timeout, nil, nil)
		require.NoError(t, err)
		err = m.validateSetFC(tx, attr, authProof, exeCtx)
		require.ErrorContains(t, err, "fee credit record owner predicate does not match the target owner predicate")
	})
}

func TestExecuteSetFC(t *testing.T) {
	// generate keys
	adminKeySigner, adminKeyVerifier := testsig.CreateSignerAndVerifier(t)
	adminPubKey, err := adminKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	_, userKeyVerifier := testsig.CreateSignerAndVerifier(t)
	userPubKey, err := userKeyVerifier.MarshalPublicKey()
	require.NoError(t, err)

	// create fee credit module
	stateTree := state.NewEmptyState()
	networkID := types.NetworkID(5)
	partitionID := types.PartitionID(5)
	fcrUnitType := []byte{1}
	adminOwnerPredicate := templates.NewP2pkh256BytesFromKey(adminPubKey)
	m, err := NewFeeCreditModule(networkID, partitionID, stateTree, fcrUnitType, adminOwnerPredicate)
	require.NoError(t, err)

	// create tx
	fcrOwnerPredicate := templates.NewP2pkh256BytesFromKey(userPubKey)
	timeout := uint64(10)
	fcrID := newFeeCreditRecordID(t, fcrOwnerPredicate, fcrUnitType, timeout)

	tx, attr, authProof, err := newSetFeeCreditTx(adminKeySigner, partitionID, fcrID, fcrOwnerPredicate, nil, timeout, nil, nil)
	require.NoError(t, err)

	// execute tx
	sm, err := m.executeSetFC(tx, attr, authProof, testctx.NewMockExecutionContext())
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
	require.EqualValues(t, 10, fcr.MinLifetime)
	require.EqualValues(t, 0, fcr.Locked)
	require.EqualValues(t, fcrOwnerPredicate, fcr.OwnerPredicate)
}

func newFeeCreditRecordID(t *testing.T, ownerPredicate []byte, fcrUnitType []byte, timeout uint64) types.UnitID {
	unitPart, err := fc.NewFeeCreditRecordUnitPart(ownerPredicate, timeout)
	require.NoError(t, err)
	return types.NewUnitID(33, nil, unitPart, fcrUnitType)
}

func newSetFeeCreditTx(adminKey crypto.Signer, partitionID types.PartitionID, unitID, fcrOwnerPredicate []byte, counter *uint64, timeout uint64, fcrID, feeProof []byte) (*types.TransactionOrder, *permissioned.SetFeeCreditAttributes, *permissioned.SetFeeCreditAuthProof, error) {
	attr := &permissioned.SetFeeCreditAttributes{
		OwnerPredicate: fcrOwnerPredicate,
		Counter:        counter,
		Amount:         1e8,
	}
	payload, err := newTxPayload(partitionID, permissioned.TransactionTypeSetFeeCredit, unitID, fcrID, timeout, nil, attr)
	if err != nil {
		return nil, nil, nil, err
	}
	txo := &types.TransactionOrder{
		Version:  1,
		Payload:  payload,
		FeeProof: feeProof,
	}
	authProof, err := signAuthProof(txo, adminKey, func(ownerProof []byte) *permissioned.SetFeeCreditAuthProof {
		return &permissioned.SetFeeCreditAuthProof{OwnerProof: ownerProof}
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return txo, attr, authProof, nil
}

func newTxPayload(partitionID types.PartitionID, txType uint16, unitID, fcrID types.UnitID, timeout uint64, refNo []byte, attr interface{}) (types.Payload, error) {
	attrBytes, err := types.Cbor.Marshal(attr)
	if err != nil {
		return types.Payload{}, err
	}
	return types.Payload{
		PartitionID: partitionID,
		Type:        txType,
		UnitID:      unitID,
		Attributes:  attrBytes,
		ClientMetadata: &types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: 10,
			FeeCreditRecordID: fcrID,
			ReferenceNumber:   refNo,
		},
	}, nil
}
