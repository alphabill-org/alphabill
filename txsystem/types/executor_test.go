package types

import (
	"errors"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
)

const mockTx = "mockTx"

type txSysInfo struct {
	getUnit      func(id types.UnitID, committed bool) (*state.Unit, error)
	currentRound func() uint64
}

type MockTxAttributes struct {
	_     struct{} `cbor:",toarray"`
	Value uint64
}

type MockModule struct {
	ValidateError error
	Result        error
}

func NewMockTxModule(wantErr error) *MockModule {
	return &MockModule{Result: wantErr}
}

func (mm MockModule) mockValidateTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ ExecutionContext) (err error) {
	return mm.ValidateError
}
func (mm MockModule) mockExecuteTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ ExecutionContext) (*types.ServerMetadata, error) {
	if mm.Result != nil {
		return nil, mm.Result
	}
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (mm MockModule) TxHandlers() map[string]TxExecutor {
	return map[string]TxExecutor{
		mockTx: NewTxHandler[MockTxAttributes](mm.mockValidateTx, mm.mockExecuteTx),
	}
}

func (s txSysInfo) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	if s.getUnit != nil {
		return s.getUnit(id, committed)
	}
	return &state.Unit{}, fmt.Errorf("unit does not exist")
}

func (s txSysInfo) CurrentRound() uint64 {
	if s.currentRound != nil {
		return s.currentRound()
	}
	return 5
}

func Test_TxExecutors_Execute(t *testing.T) {
	t.Run("validate/execute/executeWithAttr - unknown tx type", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(errors.New("unexpected call"))
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := &types.TransactionOrder{Payload: &types.Payload{Type: "bar"}}
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		// try calling validate
		require.EqualError(t, err, `unknown transaction type bar`)
		require.Nil(t, attr)
		// try calling execute with attr
		sm, err := exec.ExecuteWithAttr(txOrder, attr, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.Nil(t, sm)
		require.EqualError(t, err, `unknown transaction type bar`)
		// try to execute
		sm, err = exec.Execute(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.EqualError(t, err, "unknown transaction type bar")
		require.Nil(t, sm)
		// try calling validate and execute
		sm, err = exec.ValidateAndExecute(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.EqualError(t, err, "unknown transaction type bar")
		require.Nil(t, sm)
	})

	t.Run("tx execute with attr returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := &types.TransactionOrder{Payload: &types.Payload{Type: mockTx}}
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.EqualError(t, err, "failed to unmarshal payload: EOF")
		require.Nil(t, attr)
		// try to execute anyway
		sm, err := exec.ExecuteWithAttr(txOrder, attr, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.Nil(t, sm)
		require.EqualError(t, err, "incorrect attribute type: <nil> for tx order mockTx")
	})

	t.Run("tx execute returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := &types.TransactionOrder{Payload: &types.Payload{Type: mockTx}}
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.EqualError(t, err, "failed to unmarshal payload: EOF")
		require.Nil(t, attr)
		// try to execute anyway
		sm, err := exec.Execute(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.Nil(t, sm)
		require.EqualError(t, err, "tx order execution failed: failed to unmarshal payload: EOF")
	})

	t.Run("tx validate returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(nil)
		mock.ValidateError = expErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.ErrorIs(t, err, expErr)
		require.Nil(t, attr)
	})

	t.Run("tx validate and execute does not execute if validate fails", func(t *testing.T) {
		exec := make(TxExecutors)
		execErr := errors.New("tx execute failed")
		validateErr := errors.New("tx validate failed")
		mock := NewMockTxModule(execErr)
		mock.ValidateError = validateErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.ValidateAndExecute(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.ErrorIs(t, err, validateErr)
		require.Nil(t, attr)
	})

	t.Run("tx validate and execute, execute step fails", func(t *testing.T) {
		exec := make(TxExecutors)
		execErr := errors.New("tx execute failed")
		mock := NewMockTxModule(execErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.ValidateAndExecute(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.ErrorIs(t, err, execErr)
		require.Nil(t, attr)
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
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(TestData{Data: []byte{1, 4}}))
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.EqualError(t, err, "failed to unmarshal payload: cbor: cannot unmarshal byte string into Go struct field types.MockTxAttributes.Value of type uint64")
		require.Nil(t, attr)
	})

	t.Run("tx execute with attr returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("tx handler failed")
		mock := NewMockTxModule(expErr)
		mock.ValidateError = expErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.ErrorIs(t, err, expErr)
		require.Nil(t, attr)
		sm, err := exec.ExecuteWithAttr(txOrder, attr, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.EqualError(t, err, "incorrect attribute type: <nil> for tx order mockTx")
		require.Nil(t, sm)
	})

	t.Run("validate success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.NoError(t, err)
		require.NotNil(t, attr)
	})

	t.Run("execute success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.NoError(t, err)
		require.NotNil(t, attr)
		sm, err := exec.Execute(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("execute with attr success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		attr, err := exec.Validate(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.NoError(t, err)
		require.NotNil(t, attr)
		sm, err := exec.ExecuteWithAttr(txOrder, attr, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("validate and execute success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txOrder := transaction.NewTransactionOrder(t,
			transaction.WithPayloadType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}))
		sm, err := exec.ValidateAndExecute(txOrder, NewExecutionContext(&txSysInfo{}, nil, 10))
		require.NoError(t, err)
		require.NotNil(t, sm)
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
		require.Contains(t, dst, mockTx)
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
		require.EqualError(t, dst.Add(mock.TxHandlers()), `tx executor for "mockTx" is already registered`)
		require.Len(t, dst, 1)
		require.Contains(t, dst, mockTx)
	})

	t.Run("success", func(t *testing.T) {
		dst := make(TxExecutors)
		mock := NewMockTxModule(nil)

		require.NoError(t, dst.Add(mock.TxHandlers()))
		require.Len(t, dst, 1)
		require.Contains(t, dst, mockTx)
	})
}