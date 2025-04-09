package txsystem

import (
	"testing"

	basetemplates "github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	abfc "github.com/alphabill-org/alphabill/txsystem/fc"
	tt "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func Test_StateUnlockProofFromBytes(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		tx := &types.TransactionOrder{Version: 1, StateUnlock: nil}

		_, err := stateUnlockProofFromTx(tx)
		require.Error(t, err)
		require.Equal(t, "invalid state unlock proof: empty", err.Error())
	})

	t.Run("empty input", func(t *testing.T) {
		tx := &types.TransactionOrder{Version: 1, StateUnlock: []byte{}}

		_, err := stateUnlockProofFromTx(tx)
		require.Error(t, err)
		require.Equal(t, "invalid state unlock proof: empty", err.Error())
	})

	t.Run("valid input execute kind", func(t *testing.T) {
		kind := StateUnlockExecute
		proof := []byte("proof")

		tx := &types.TransactionOrder{Version: 1, StateUnlock: append([]byte{byte(kind)}, proof...)}
		result, err := stateUnlockProofFromTx(tx)
		require.NoError(t, err)
		require.Equal(t, kind, result.Kind)
		require.Equal(t, proof, result.Proof)
	})

	t.Run("valid input rollback kind", func(t *testing.T) {
		kind := StateUnlockRollback
		proof := []byte("proof")

		tx := &types.TransactionOrder{Version: 1, StateUnlock: append([]byte{byte(kind)}, proof...)}
		result, err := stateUnlockProofFromTx(tx)
		require.NoError(t, err)
		require.Equal(t, kind, result.Kind)
		require.Equal(t, proof, result.Proof)
	})

	t.Run("invalid kind", func(t *testing.T) {
		kind := byte(2) // Invalid kind
		proof := []byte("proof")
		tx := &types.TransactionOrder{Version: 1, StateUnlock: append([]byte{kind}, proof...)}

		result, err := stateUnlockProofFromTx(tx)
		require.NoError(t, err)
		require.NotEqual(t, StateUnlockExecute, result.Kind)
		require.NotEqual(t, StateUnlockRollback, result.Kind)
		require.Equal(t, proof, result.Proof)
	})
}

func Test_proof_check_with_nil(t *testing.T) {
	kind := StateUnlockExecute
	proof := []byte("proof")
	tx := &types.TransactionOrder{Version: 1, StateUnlock: append([]byte{byte(kind)}, proof...)}
	result, err := stateUnlockProofFromTx(tx)
	require.NoError(t, err)
	templEng, err := templates.New(observability.Default(t))
	require.NoError(t, err)
	predEng, err := predicates.Dispatcher(templEng)
	require.NoError(t, err)
	predicateRunner := predicates.NewPredicateRunner(predEng.Execute)
	require.EqualError(t, result.check(predicateRunner, nil, nil, nil), "StateLock is nil")
}

func TestGenericTxSystem_handleUnlockUnitState(t *testing.T) {
	t.Run("ok - unit not found", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil)
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
		)
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.NoError(t, err)
		require.Nil(t, sm)
	})
	t.Run("ok - unit is already unlocked", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
		)
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.NoError(t, err)
		require.Nil(t, sm)
	})
	t.Run("err - try to manipulate locked item without unlocking", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			&money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()},
			createLockTransaction(t, unitID, pubKey1)))
		// try to transfer without unlocking
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
		)
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.EqualError(t, err, "unlock proof error: invalid state unlock proof: empty")
		require.Nil(t, sm)
	})
	t.Run("err - unlock fails invalid kind", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			&money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()},
			createLockTransaction(t, unitID, pubKey1)))
		// add unlock
		require.NoError(t, err)
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
			tt.WithStateUnlock([]byte{255}),
		)
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.EqualError(t, err, "unlock error: invalid state unlock proof kind")
		require.Nil(t, sm)
	})
	t.Run("err - execute verify fails", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := moneyid.NewBillID(t)
		targetPDR := moneyid.PDR()
		fcrID, err := targetPDR.ComposeUnitID(types.ShardID{}, 0xff, moneyid.Random)
		require.NoError(t, err)
		txSys := NewTestGenericTxSystem(t, nil,
			withStateUnit(fcrID, &fcsdk.FeeCreditRecord{Balance: 10, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil),
			withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()},
				createLockTransaction(t, unitID, pubKey1)),
		)
		// add unlock
		require.NoError(t, err)
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
			tt.WithStateUnlock([]byte{byte(StateUnlockExecute), 1, 2, 3}),
			tt.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				FeeCreditRecordID: fcrID,
				MaxTransactionFee: 10,
			}))
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.EqualError(t, err, "unlock error: state lock's execution predicate failed: executing predicate: failed to decode P2PKH256 signature: cbor: 2 bytes of extraneous data starting at index 1")
		require.Nil(t, sm)
	})
	t.Run("err - rollback verify fails", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			&money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()},
			createLockTransaction(t, unitID, pubKey1)))
		// add unlock
		require.NoError(t, err)
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
			tt.WithStateUnlock([]byte{byte(StateUnlockRollback), 1, 2, 3}),
		)
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.EqualError(t, err, "unlock error: state lock's rollback predicate failed: predicate is empty")
		require.Nil(t, sm)
	})
	t.Run("err - unlock succeeds, but Tx does not", func(t *testing.T) {
		sig1, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			&money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()},
			createLockTransaction(t, unitID, pubKey1)))
		// add unlock
		require.NoError(t, err)
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
			tt.WithAuthProof(&money.TransferAuthProof{}),
		)
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)

		ownerProof := testsig.NewStateLockProofSignature(t, tx, sig1)
		tx.StateUnlock = append([]byte{byte(StateUnlockExecute)}, ownerProof...)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.EqualError(t, err, "failed to execute transaction that was on hold: unknown transaction type 1")
		require.Nil(t, sm)
	})
	t.Run("ok - unlock and Tx succeed", func(t *testing.T) {
		// create state with unit(unitID, lockedFor=pubKey1)
		sig1, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := moneyid.NewBillID(t)
		dummyUnitID1 := moneyid.NewBillID(t)
		dummyUnitID2 := moneyid.NewBillID(t)

		// create "txOnHold" with 3 target units
		targetUnits := []types.UnitID{unitID, dummyUnitID1, dummyUnitID2}
		txOnHold := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(mockSplitTxType),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&MockSplitTxAttributes{Value: 10, TargetUnits: targetUnits}),
			tt.WithAuthProof(&MockTxAuthProof{}),
			tt.WithStateLock(&types.StateLock{
				ExecutionPredicate: basetemplates.NewP2pkh256BytesFromKey(pubKey1),
				RollbackPredicate:  basetemplates.NewP2pkh256BytesFromKey(pubKey1)},
			),
		)
		txOnHoldBytes, err := cbor.Marshal(txOnHold)
		require.NoError(t, err)

		// create mock tx system with unit(unitID, txOnHold) and dummy target units
		txSys := NewTestGenericTxSystem(t,
			[]txtypes.Module{NewMockTxModule(nil)},
			withStateUnit(
				unitID,
				&money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()},
				txOnHoldBytes,
			),
			withStateUnit(
				dummyUnitID1,
				nil,
				txOnHoldBytes,
			),
			withStateUnit(
				dummyUnitID2,
				nil,
				txOnHoldBytes,
			),
		)

		// create the rollback tx
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(mockTxType),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&MockTxAttributes{}),
			tt.WithAuthProof(&MockTxAuthProof{}),
		)
		stateUnlockProof := testsig.NewStateLockProofSignature(t, tx, sig1)
		tx.StateUnlock = append([]byte{byte(StateUnlockRollback)}, stateUnlockProof...)

		// execute the rollback tx
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, unitID, execCtx)
		require.NoError(t, err)
		require.NotNil(t, sm)
		require.Equal(t, unitID, sm.TargetUnits[0])

		// verify state lock was removed for all units and the dummy units are deleted
		u, err := txSys.GetUnit(unitID, false)
		require.NoError(t, err)
		unit, err := state.ToUnitV1(u)
		require.NoError(t, err)
		require.NotNil(t, unit)
		require.False(t, unit.IsStateLocked())
		require.Zero(t, unit.DeletionRound())

		dummyUnit1, err := txSys.GetUnit(dummyUnitID1, false)
		require.NoError(t, err)
		require.NotNil(t, dummyUnit1)
		dummyUnit1V1, err := state.ToUnitV1(dummyUnit1)
		require.NoError(t, err)
		require.False(t, false, dummyUnit1V1.IsStateLocked())
		require.EqualValues(t, 1, dummyUnit1V1.DeletionRound())

		dummyUnit2, err := txSys.GetUnit(dummyUnitID2, false)
		require.NoError(t, err)
		dummyUnit2V1, err := state.ToUnitV1(dummyUnit2)
		require.NoError(t, err)
		require.NotNil(t, dummyUnit2V1)
		require.False(t, false, dummyUnit2V1.IsStateLocked())
		require.EqualValues(t, 1, dummyUnit2V1.DeletionRound())
	})
}

func TestGenericTxSystem_executeLockUnitState(t *testing.T) {
	t.Run("err - invalid state lock", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
			tt.WithStateLock(&types.StateLock{}),
		)
		txBytes, err := types.Cbor.Marshal(tx)
		require.NoError(t, err)
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.executeLockUnitState(tx, txBytes, []types.UnitID{unitID}, execCtx)
		require.EqualError(t, err, "invalid state lock parameter: missing execution predicate")
		require.Nil(t, sm)
	})
	t.Run("err - invalid state lock, missing rollback", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
			tt.WithStateLock(&types.StateLock{ExecutionPredicate: []byte{1, 2, 3}}),
		)
		txBytes, err := types.Cbor.Marshal(tx)
		require.NoError(t, err)
		targetUnits := []types.UnitID{unitID}
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.executeLockUnitState(tx, txBytes, targetUnits, execCtx)
		require.EqualError(t, err, "invalid state lock parameter: missing rollback predicate")
		require.Nil(t, sm)
	})
	t.Run("ok", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeTransfer),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(&money.TransferAttributes{}),
			tt.WithStateLock(&types.StateLock{
				ExecutionPredicate: basetemplates.AlwaysTrueBytes(),
				RollbackPredicate:  basetemplates.AlwaysTrueBytes(),
			}),
		)
		txBytes, err := types.Cbor.Marshal(tx)
		require.NoError(t, err)
		targetUnits := []types.UnitID{unitID}
		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.executeLockUnitState(tx, txBytes, targetUnits, execCtx)
		require.NoError(t, err, "invalid state lock parameter: missing rollback predicate")
		require.NotNil(t, sm)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		// verify unit got locked
		u, err := txSys.state.GetUnit(unitID, false)
		require.NoError(t, err)
		unit, err := state.ToUnitV1(u)
		require.NoError(t, err)
		require.True(t, unit.IsStateLocked())
	})
	t.Run("ok - lock state with multiple target units", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 10, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		attr := &money.SplitAttributes{
			TargetUnits: []*money.TargetUnit{
				{Amount: 1, OwnerPredicate: nil},
				{Amount: 2, OwnerPredicate: nil},
			},
		}
		tx := tt.NewTransactionOrder(
			t,
			tt.WithTransactionType(money.TransactionTypeSplit),
			tt.WithUnitID(unitID),
			tt.WithPartitionID(money.DefaultPartitionID),
			tt.WithAttributes(attr),
			tt.WithAuthProof(&money.SplitAuthProof{
				OwnerProof: basetemplates.AlwaysTrueBytes(),
			}),
			tt.WithStateLock(&types.StateLock{
				ExecutionPredicate: basetemplates.AlwaysTrueBytes(),
				RollbackPredicate:  basetemplates.AlwaysTrueBytes(),
			}),
		)
		txBytes, err := types.Cbor.Marshal(tx)
		require.NoError(t, err)
		targetUnits := []types.UnitID{unitID}
		idGen := money.PrndSh(tx)
		for range attr.TargetUnits {
			newUnitID, err := txSys.pdr.ComposeUnitID(types.ShardID{}, money.BillUnitType, idGen)
			require.NoError(t, err)
			targetUnits = append(targetUnits, newUnitID)
		}

		execCtx := txtypes.NewExecutionContext(txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.executeLockUnitState(tx, txBytes, targetUnits, execCtx)
		require.NoError(t, err)
		require.NotNil(t, sm)
		require.Equal(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		require.Equal(t, targetUnits, sm.TargetUnits)

		// verify main unit got locked
		u, err := txSys.state.GetUnit(unitID, false)
		require.NoError(t, err)
		unit, err := state.ToUnitV1(u)
		require.NoError(t, err)
		require.Equal(t, txBytes, unit.StateLockTx())

		// and dummy units were created for targets
		require.Len(t, targetUnits, 3)
		for i := 1; i < len(targetUnits); i++ {
			u, err = txSys.state.GetUnit(targetUnits[i], false)
			require.NoError(t, err)
			require.Nil(t, u.Data())
			unit, err := state.ToUnitV1(u)
			require.NoError(t, err)
			require.Equal(t, txBytes, unit.StateLockTx())
		}
	})
}

func createLockTransaction(t *testing.T, id types.UnitID, pubkey []byte) []byte {
	tx := tt.NewTransactionOrder(
		t,
		tt.WithTransactionType(money.TransactionTypeTransfer),
		tt.WithUnitID(id),
		tt.WithPartitionID(money.DefaultPartitionID),
		tt.WithAttributes(&money.TransferAttributes{NewOwnerPredicate: basetemplates.AlwaysTrueBytes(), TargetValue: 1, Counter: 1}),
		tt.WithStateLock(&types.StateLock{
			ExecutionPredicate: basetemplates.NewP2pkh256BytesFromKey(pubkey)}),
	)
	txBytes, err := cbor.Marshal(tx)
	require.NoError(t, err)
	return txBytes
}
