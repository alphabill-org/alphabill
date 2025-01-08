package txsystem

import (
	"testing"

	"github.com/alphabill-org/alphabill/state"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	basetemplates "github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	abfc "github.com/alphabill-org/alphabill/txsystem/fc"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
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
	predEng, err := predicates.Dispatcher(templates.New())
	require.NoError(t, err)
	predicateRunner := predicates.NewPredicateRunner(predEng.Execute)
	require.EqualError(t, result.check(predicateRunner, nil, nil, nil), "StateLock is nil")
}

func TestGenericTxSystem_handleUnlockUnitState(t *testing.T) {
	t.Run("ok - unit not found", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil)
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.NoError(t, err)
		require.Nil(t, sm)
	})
	t.Run("ok - unit is already unlocked", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.NoError(t, err)
		require.Nil(t, sm)
	})
	t.Run("ok - unit is already unlocked", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
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
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
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
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithUnlockProof([]byte{255}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
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
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithUnlockProof([]byte{byte(StateUnlockExecute), 1, 2, 3}),
			testtransaction.WithClientMetadata(&types.ClientMetadata{
				Timeout:           txSys.currentRoundNumber + 1,
				FeeCreditRecordID: fcrID,
				MaxTransactionFee: 10,
			}))
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
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
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithUnlockProof([]byte{byte(StateUnlockRollback), 1, 2, 3}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
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
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithAuthProof(&money.TransferAuthProof{}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)

		ownerProof := testsig.NewStateLockProofSignature(t, tx, sig1)
		tx.StateUnlock = append([]byte{byte(StateUnlockExecute)}, ownerProof...)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.EqualError(t, err, "failed to execute transaction that was on hold: unknown transaction type 1")
		require.Nil(t, sm)
	})
}

func TestGenericTxSystem_executeLockUnitState(t *testing.T) {
	t.Run("err - invalid state lock", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithStateLock(&types.StateLock{}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.executeLockUnitState(tx, execCtx)
		require.EqualError(t, err, "invalid state lock parameter: missing execution predicate")
		require.Nil(t, sm)
	})
	t.Run("err - invalid state lock, missing rollback", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithStateLock(&types.StateLock{ExecutionPredicate: []byte{1, 2, 3}}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.executeLockUnitState(tx, execCtx)
		require.EqualError(t, err, "invalid state lock parameter: missing rollback predicate")
		require.Nil(t, sm)
	})
	t.Run("ok", func(t *testing.T) {
		unitID := moneyid.NewBillID(t)
		txSys := NewTestGenericTxSystem(t, nil, withStateUnit(unitID, &money.BillData{Value: 1, Counter: 1, OwnerPredicate: basetemplates.AlwaysTrueBytes()}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithTransactionType(money.TransactionTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithPartitionID(money.DefaultPartitionID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: basetemplates.AlwaysTrueBytes(),
				RollbackPredicate:  basetemplates.AlwaysTrueBytes(),
			}),
		)
		execCtx := txtypes.NewExecutionContext(tx, txSys, abfc.NewNoFeeCreditModule(), nil, 10)
		sm, err := txSys.executeLockUnitState(tx, execCtx)
		require.NoError(t, err, "invalid state lock parameter: missing rollback predicate")
		require.NotNil(t, sm)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		// verify unit got locked
		u, err := txSys.state.GetUnit(unitID, false)
		require.NoError(t, err)
		require.True(t, state.UnitV1(u).IsStateLocked())
	})
}

func createLockTransaction(t *testing.T, id types.UnitID, pubkey []byte) []byte {
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithTransactionType(money.TransactionTypeTransfer),
		testtransaction.WithUnitID(id),
		testtransaction.WithPartitionID(money.DefaultPartitionID),
		testtransaction.WithAttributes(&money.TransferAttributes{NewOwnerPredicate: basetemplates.AlwaysTrueBytes(), TargetValue: 1, Counter: 1}),
		testtransaction.WithStateLock(&types.StateLock{
			ExecutionPredicate: basetemplates.NewP2pkh256BytesFromKey(pubkey)}),
	)
	txBytes, err := cbor.Marshal(tx)
	require.NoError(t, err)
	return txBytes
}
