package txsystem

import (
	"testing"

	basetemplates "github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func Test_StateUnlockProofFromBytes(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		tx := &types.TransactionOrder{StateUnlock: nil}

		_, err := StateUnlockProofFromTx(tx)
		require.Error(t, err)
		require.Equal(t, "invalid state unlock proof: empty", err.Error())
	})

	t.Run("empty input", func(t *testing.T) {
		tx := &types.TransactionOrder{StateUnlock: []byte{}}

		_, err := StateUnlockProofFromTx(tx)
		require.Error(t, err)
		require.Equal(t, "invalid state unlock proof: empty", err.Error())
	})

	t.Run("valid input execute kind", func(t *testing.T) {
		kind := StateUnlockExecute
		proof := []byte("proof")

		tx := &types.TransactionOrder{StateUnlock: append([]byte{byte(kind)}, proof...)}
		result, err := StateUnlockProofFromTx(tx)
		require.NoError(t, err)
		require.Equal(t, kind, result.Kind)
		require.Equal(t, proof, result.Proof)
	})

	t.Run("valid input rollback kind", func(t *testing.T) {
		kind := StateUnlockRollback
		proof := []byte("proof")

		tx := &types.TransactionOrder{StateUnlock: append([]byte{byte(kind)}, proof...)}
		result, err := StateUnlockProofFromTx(tx)
		require.NoError(t, err)
		require.Equal(t, kind, result.Kind)
		require.Equal(t, proof, result.Proof)
	})

	t.Run("invalid kind", func(t *testing.T) {
		kind := byte(2) // Invalid kind
		proof := []byte("proof")
		tx := &types.TransactionOrder{StateUnlock: append([]byte{kind}, proof...)}

		result, err := StateUnlockProofFromTx(tx)
		require.NoError(t, err)
		require.NotEqual(t, StateUnlockExecute, result.Kind)
		require.NotEqual(t, StateUnlockRollback, result.Kind)
		require.Equal(t, proof, result.Proof)
	})
}

func Test_proof_check_with_nil(t *testing.T) {
	kind := StateUnlockExecute
	proof := []byte("proof")
	tx := &types.TransactionOrder{StateUnlock: append([]byte{byte(kind)}, proof...)}
	result, err := StateUnlockProofFromTx(tx)
	require.NoError(t, err)
	engines, err := predicates.Dispatcher(templates.New())
	require.NoError(t, err)
	s := state.NewEmptyState()
	predicateRunner := predicates.NewPredicateRunner(engines.Execute, s)
	require.EqualError(t, result.check(predicateRunner, nil, nil), "StateLock is nil")
}

func TestGenericTxSystem_handleUnlockUnitState(t *testing.T) {
	t.Run("ok - unit not found", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil)
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.NoError(t, err)
		require.Nil(t, sm)
	})
	t.Run("ok - unit is already unlocked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(unitID, basetemplates.AlwaysTrueBytes(), &money.BillData{V: 1, Counter: 1}, nil))
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.NoError(t, err)
		require.Nil(t, sm)
	})
	t.Run("ok - unit is already unlocked", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(unitID, basetemplates.AlwaysTrueBytes(), &money.BillData{V: 1, Counter: 1}, nil))
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.NoError(t, err)
		require.Nil(t, sm)
	})
	t.Run("err - try to manipulate locked item without unlocking", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			basetemplates.AlwaysTrueBytes(),
			&money.BillData{V: 1, Counter: 1},
			createLockTransaction(t, unitID, pubKey1)))
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		// try to transfer without unlocking
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.EqualError(t, err, "unlock proof error: invalid state unlock proof: empty")
		require.Nil(t, sm)
	})
	t.Run("err - unlock fails invalid kind", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			basetemplates.AlwaysTrueBytes(),
			&money.BillData{V: 1, Counter: 1},
			createLockTransaction(t, unitID, pubKey1)))
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		// add unlock
		require.NoError(t, err)
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithUnlockProof([]byte{255}),
		)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.EqualError(t, err, "unlock error: invalid state unlock proof kind")
		require.Nil(t, sm)
	})
	t.Run("err - execute verify fails", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			basetemplates.AlwaysTrueBytes(),
			&money.BillData{V: 1, Counter: 1},
			createLockTransaction(t, unitID, pubKey1)))
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		// add unlock
		require.NoError(t, err)
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithUnlockProof([]byte{byte(StateUnlockExecute), 1, 2, 3}),
		)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.EqualError(t, err, "unlock error: state lock's execution predicate failed: executing predicate: failed to decode P2PKH256 signature: cbor: 2 bytes of extraneous data starting at index 1")
		require.Nil(t, sm)
	})
	t.Run("err - rollback verify fails", func(t *testing.T) {
		_, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			basetemplates.AlwaysTrueBytes(),
			&money.BillData{V: 1, Counter: 1},
			createLockTransaction(t, unitID, pubKey1)))
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		// add unlock
		require.NoError(t, err)
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithUnlockProof([]byte{byte(StateUnlockRollback), 1, 2, 3}),
		)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.EqualError(t, err, "unlock error: state lock's rollback predicate failed: predicate is empty")
		require.Nil(t, sm)
	})
	t.Run("err - unlock succeeds, but Tx does not", func(t *testing.T) {
		sig1, ver1 := testsig.CreateSignerAndVerifier(t)
		pubKey1, err := ver1.MarshalPublicKey()
		require.NoError(t, err)
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(
			unitID,
			basetemplates.AlwaysTrueBytes(),
			&money.BillData{V: 1, Counter: 1},
			createLockTransaction(t, unitID, pubKey1)))
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		// add unlock
		require.NoError(t, err)
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
		)
		data, err := tx.PayloadBytes()
		require.NoError(t, err)
		unlockProof, err := predicates.OwnerProoferForSigner(sig1)(data)
		require.NoError(t, err)
		// update unlock
		tx.StateUnlock = append([]byte{byte(StateUnlockExecute)}, unlockProof...)
		sm, err := txSys.handleUnlockUnitState(tx, execCtx)
		require.EqualError(t, err, "failed to execute tx that was on hold: unknown transaction type trans")
		require.Nil(t, sm)
	})
}

func TestGenericTxSystem_executeLockUnitState(t *testing.T) {
	t.Run("err - invalid state lock", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(unitID, basetemplates.AlwaysTrueBytes(), &money.BillData{V: 1, Counter: 1}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithStateLock(&types.StateLock{}),
		)
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		sm, err := txSys.executeLockUnitState(tx, execCtx)
		require.EqualError(t, err, "invalid state lock parameter: missing execution predicate")
		require.Nil(t, sm)
	})
	t.Run("err - invalid state lock, missing rollback", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(unitID, basetemplates.AlwaysTrueBytes(), &money.BillData{V: 1, Counter: 1}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithStateLock(&types.StateLock{ExecutionPredicate: []byte{1, 2, 3}}),
		)
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		sm, err := txSys.executeLockUnitState(tx, execCtx)
		require.EqualError(t, err, "invalid state lock parameter: missing rollback predicate")
		require.Nil(t, sm)
	})
	t.Run("ok", func(t *testing.T) {
		unitID := money.NewBillID(nil, []byte{2})
		txSys := newTestGenericTxSystem(t, nil, withStateUnit(unitID, basetemplates.AlwaysTrueBytes(), &money.BillData{V: 1, Counter: 1}, nil))
		tx := testtransaction.NewTransactionOrder(
			t,
			testtransaction.WithPayloadType(money.PayloadTypeTransfer),
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(money.DefaultSystemID),
			testtransaction.WithAttributes(&money.TransferAttributes{}),
			testtransaction.WithStateLock(&types.StateLock{
				ExecutionPredicate: basetemplates.AlwaysTrueBytes(),
				RollbackPredicate:  basetemplates.AlwaysTrueBytes(),
			}),
		)
		execCtx := &TxExecutionContext{CurrentBlockNr: 6}
		sm, err := txSys.executeLockUnitState(tx, execCtx)
		require.NoError(t, err, "invalid state lock parameter: missing rollback predicate")
		require.NotNil(t, sm)
		require.EqualValues(t, []types.UnitID{unitID}, sm.TargetUnits)
		require.EqualValues(t, types.TxStatusSuccessful, sm.SuccessIndicator)
		// verify unit got locked
		u, err := txSys.state.GetUnit(unitID, false)
		require.NoError(t, err)
		require.True(t, u.IsStateLocked())
	})
}

func createLockTransaction(t *testing.T, id types.UnitID, pubkey []byte) []byte {
	tx := testtransaction.NewTransactionOrder(
		t,
		testtransaction.WithPayloadType(money.PayloadTypeTransfer),
		testtransaction.WithUnitID(id),
		testtransaction.WithSystemID(money.DefaultSystemID),
		testtransaction.WithAttributes(&money.TransferAttributes{NewBearer: basetemplates.AlwaysTrueBytes(), TargetValue: 1, Counter: 1}),
		testtransaction.WithStateLock(&types.StateLock{
			ExecutionPredicate: basetemplates.NewP2pkh256BytesFromKey(pubkey)}),
	)
	txBytes, err := cbor.Marshal(tx)
	require.NoError(t, err)
	return txBytes
}

type txSystemTestOption func(m *GenericTxSystem) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData, lock []byte) txSystemTestOption {
	return func(m *GenericTxSystem) error {
		return m.state.Apply(state.AddUnitWithLock(unitID, bearer, data, lock))
	}
}

func newTestGenericTxSystem(t *testing.T, modules []Module, opts ...txSystemTestOption) *GenericTxSystem {
	txSys := defaultTestConfiguration(t, modules)
	// apply test overrides
	for _, opt := range opts {
		require.NoError(t, opt(txSys))
	}
	return txSys
}

func defaultTestConfiguration(t *testing.T, modules []Module) *GenericTxSystem {
	txSys, err := NewGenericTxSystem(types.SystemID(1), nil, modules, observability.Default(t))
	require.NoError(t, err)
	return txSys
}
