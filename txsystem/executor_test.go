package txsystem

import (
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
)

func Test_TxExecutors_Execute(t *testing.T) {
	t.Run("unknown tx type", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(errors.New("unexpected call"))
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := &types.TransactionOrder{Payload: &types.Payload{Type: "bar"}}
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.EqualError(t, err, `unknown transaction type bar`)
		require.Nil(t, attr)
		// try to execute anyway
		sm, err := exec.ExecuteWithAttr(txOrder, attr, &TxExecutionContext{CurrentBlockNr: 5})
		require.Nil(t, sm)
		require.EqualError(t, err, `unknown transaction type bar`)
	})

	t.Run("tx handler returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := &types.TransactionOrder{Payload: &types.Payload{Type: mockTxType}}
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.EqualError(t, err, "failed to unmarshal payload: EOF")
		require.Nil(t, attr)
		// try to execute anyway
		sm, err := exec.ExecuteWithAttr(txOrder, attr, &TxExecutionContext{CurrentBlockNr: 5})
		require.Nil(t, sm)
		require.EqualError(t, err, "incorrect attribute type: <nil> for tx order mockTx-type")
	})

	t.Run("tx handler validate error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		mock.ValidateError = expErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.ErrorIs(t, err, expErr)
		require.Nil(t, attr)
	})

	t.Run("tx handler validate ok", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.NoError(t, err)
		require.NotNil(t, attr)
	})

	t.Run("tx handler validate, incorrect attributes", func(t *testing.T) {
		type TestData struct {
			_    struct{} `cbor:",toarray"`
			Data []byte
		}
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(TestData{Data: []byte{1, 4}}))
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.EqualError(t, err, "failed to unmarshal payload: cbor: cannot unmarshal byte string into Go struct field txsystem.MockTxAttributes.Value of type uint64")
		require.Nil(t, attr)
	})

	t.Run("tx handler validate not found", func(t *testing.T) {
		type TestData struct {
			_   struct{} `cbor:",toarray"`
			Foo uint64
		}
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType("test"),
			transaction.WithAttributes(TestData{Foo: 1}))
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.EqualError(t, err, "unknown transaction type test")
		require.Nil(t, attr)
	})

	t.Run("tx handler returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		mock.ValidateError = expErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.ErrorIs(t, err, expErr)
		require.Nil(t, attr)
		sm, err := exec.ExecuteWithAttr(txOrder, attr, &TxExecutionContext{CurrentBlockNr: 5})
		require.EqualError(t, err, "incorrect attribute type: <nil> for tx order mockTx-type")
		require.Nil(t, sm)
	})

	t.Run("success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.NoError(t, err)
		require.NotNil(t, attr)
		sm, err := exec.ExecuteWithAttr(txOrder, attr, &TxExecutionContext{CurrentBlockNr: 5})
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("validate and execute ok", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTxType),
			transaction.WithAttributes(MockTxAttributes{}))
		sm, err := exec.ValidateAndExecute(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.ErrorIs(t, err, expErr)
		require.Nil(t, sm)
	})

	t.Run("validate and execute - no tx handler", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType("test"),
			transaction.WithAttributes(MockTxAttributes{}))
		sm, err := exec.ValidateAndExecute(txOrder, &TxExecutionContext{CurrentBlockNr: 5})
		require.EqualError(t, err, "unknown transaction type test")
		require.Nil(t, sm)
	})
}

func Test_TxExecutors_Add(t *testing.T) {
	t.Run("empty inputs", func(t *testing.T) {
		dst := make(TxExecutors)
		// both source and destinations are empty
		require.NoError(t, dst.Add(nil))
		require.Empty(t, dst)

		require.NoError(t, dst.Add(make(TxExecutors)))
		require.Empty(t, dst)
		mock := NewMockTxModule(nil)

		// when destination is not empty adding empty source to it mustn't change it
		require.NoError(t, dst.Add(mock.TxHandlers()))
		require.NoError(t, dst.Add(make(TxExecutors)))
		require.Len(t, dst, 1)
		require.Contains(t, dst, mockTxType)
	})

	t.Run("attempt to add invalid items", func(t *testing.T) {
		dst := make(TxExecutors)
		err := dst.Add(TxExecutors{"": nil})
		require.EqualError(t, err, `tx executor must have non-empty tx type name`)
		require.Empty(t, dst)

		err = dst.Add(TxExecutors{"foo": nil})
		require.EqualError(t, err, `tx executor must not be nil (foo)`)
		require.Empty(t, dst)
	})

	t.Run("adding item with the same name", func(t *testing.T) {
		dst := make(TxExecutors)
		mock := NewMockTxModule(nil)

		require.NoError(t, dst.Add(mock.TxHandlers()))
		require.EqualError(t, dst.Add(mock.TxHandlers()), `tx executor for "mockTx-type" is already registered`)
		require.Len(t, dst, 1)
		require.Contains(t, dst, mockTxType)
	})

	t.Run("success", func(t *testing.T) {
		dst := make(TxExecutors)
		mock := NewMockTxModule(nil)

		require.NoError(t, dst.Add(mock.TxHandlers()))
		require.Len(t, dst, 1)
		require.Contains(t, dst, mockTxType)
	})
}
