package types

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
)

const mockTx uint16 = 22

type txSysInfo struct {
	getUnit      func(id types.UnitID, committed bool) (state.VersionedUnit, error)
	currentRound func() uint64
}

type MockTxAttributes struct {
	_     struct{} `cbor:",toarray"`
	Value uint64
}

type MockTxAuthProof struct {
	_          struct{} `cbor:",toarray"`
	OwnerProof []byte
}

type MockModule struct {
	ValidateError error
	Result        error
}

type MockFeeHandling struct{}

func NewMockTxModule(wantErr error) *MockModule {
	return &MockModule{Result: wantErr}
}

func (mm MockModule) mockValidateTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ *MockTxAuthProof, _ ExecutionContext) (err error) {
	return mm.ValidateError
}
func (mm MockModule) mockExecuteTx(tx *types.TransactionOrder, _ *MockTxAttributes, _ *MockTxAuthProof, _ ExecutionContext) (*types.ServerMetadata, error) {
	if mm.Result != nil {
		return nil, mm.Result
	}
	return &types.ServerMetadata{ActualFee: 0, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (mm MockModule) TxHandlers() map[uint16]TxExecutor {
	return map[uint16]TxExecutor{
		mockTx: NewTxHandler[MockTxAttributes](mm.mockValidateTx, mm.mockExecuteTx),
	}
}

func NewMockFeeModule() *MockFeeHandling {
	return &MockFeeHandling{}
}

func (f *MockFeeHandling) CalculateCost(_ uint64) uint64 {
	return 0
}

func (f *MockFeeHandling) BuyGas(_ uint64) uint64 {
	return math.MaxUint64
}

func (s txSysInfo) GetUnit(id types.UnitID, committed bool) (state.VersionedUnit, error) {
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
	t.Run("validate/execute/executeWithAttr - unknown transaction type", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(errors.New("unexpected call"))
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := &types.TransactionOrder{Version: 1, Payload: types.Payload{Type: 23}}
		attr, authProof, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		// try calling validate
		require.EqualError(t, err, `unknown transaction type 23`)
		require.Nil(t, attr)
		// try calling execute with attr
		sm, err := exec.ExecuteWithAttr(txo, attr, authProof, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.Nil(t, sm)
		require.EqualError(t, err, `unknown transaction type 23`)
		// try to execute
		sm, err = exec.Execute(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.EqualError(t, err, "unknown transaction type 23")
		require.Nil(t, sm)
		// try calling validate and execute
		sm, err = exec.ValidateAndExecute(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.EqualError(t, err, "unknown transaction type 23")
		require.Nil(t, sm)
	})

	t.Run("transaction execute with attr returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("transaction handler failed")
		mock := NewMockTxModule(expErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := &types.TransactionOrder{Version: 1, Payload: types.Payload{Type: mockTx}}
		attr, authProof, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.EqualError(t, err, "failed to unmarshal payload: EOF")
		require.Nil(t, attr)
		// try to execute anyway
		sm, err := exec.ExecuteWithAttr(txo, attr, authProof, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.Nil(t, sm)
		require.EqualError(t, err, "incorrect attribute type: <nil> for transaction order 22")
	})

	t.Run("transaction execute returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("transaction handler failed")
		mock := NewMockTxModule(expErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := &types.TransactionOrder{Version: 1, Payload: types.Payload{Type: mockTx}}
		attr, _, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.EqualError(t, err, "failed to unmarshal payload: EOF")
		require.Nil(t, attr)
		// try to execute anyway
		sm, err := exec.Execute(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.Nil(t, sm)
		require.EqualError(t, err, "transaction order execution failed: failed to unmarshal payload: EOF")
	})

	t.Run("transaction validate returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("transaction handler failed")
		mock := NewMockTxModule(nil)
		mock.ValidateError = expErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		attr, _, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.ErrorIs(t, err, expErr)
		require.Nil(t, attr)
	})

	t.Run("transaction validate and execute does not execute if validate fails", func(t *testing.T) {
		exec := make(TxExecutors)
		execErr := errors.New("transaction execute failed")
		validateErr := errors.New("transaction validate failed")
		mock := NewMockTxModule(execErr)
		mock.ValidateError = validateErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		attr, err := exec.ValidateAndExecute(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.ErrorIs(t, err, validateErr)
		require.Nil(t, attr)
	})

	t.Run("transaction validate and execute, execute step fails", func(t *testing.T) {
		exec := make(TxExecutors)
		execErr := errors.New("transaction execute failed")
		mock := NewMockTxModule(execErr)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		attr, err := exec.ValidateAndExecute(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.ErrorIs(t, err, execErr)
		require.Nil(t, attr)
	})

	t.Run("transaction handler validate, incorrect attributes", func(t *testing.T) {
		type TestData struct {
			_    struct{} `cbor:",toarray"`
			Data []byte
		}
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(TestData{Data: []byte{1, 4}}))
		attr, _, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.EqualError(t, err, "failed to unmarshal payload: cbor: cannot unmarshal byte string into Go struct field types.MockTxAttributes.Value of type uint64")
		require.Nil(t, attr)
	})

	t.Run("transaction execute with attr returns error", func(t *testing.T) {
		exec := make(TxExecutors)
		expErr := errors.New("transaction handler failed")
		mock := NewMockTxModule(expErr)
		mock.ValidateError = expErr
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		attr, authProof, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.ErrorIs(t, err, expErr)
		require.Nil(t, attr)
		sm, err := exec.ExecuteWithAttr(txo, attr, authProof, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.EqualError(t, err, "incorrect attribute type: <nil> for transaction order 22")
		require.Nil(t, sm)
	})

	t.Run("validate success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		attr, _, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.NoError(t, err)
		require.NotNil(t, attr)
	})

	t.Run("execute success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		attr, _, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.NoError(t, err)
		require.NotNil(t, attr)
		sm, err := exec.Execute(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("execute with attr success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		attr, authProof, err := exec.Validate(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.NoError(t, err)
		require.NotNil(t, attr)
		sm, err := exec.ExecuteWithAttr(txo, attr, authProof, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
		require.NoError(t, err)
		require.NotNil(t, sm)
	})

	t.Run("validate and execute success", func(t *testing.T) {
		exec := make(TxExecutors)
		mock := NewMockTxModule(nil)
		require.NoError(t, exec.Add(mock.TxHandlers()))
		txo := transaction.NewTransactionOrder(t,
			transaction.WithTransactionType(mockTx),
			transaction.WithAttributes(MockTxAttributes{}),
			transaction.WithAuthProof(MockTxAuthProof{}),
		)
		sm, err := exec.ValidateAndExecute(txo, NewExecutionContext(txo, &txSysInfo{}, NewMockFeeModule(), nil, 10))
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
		err := dst.Add(TxExecutors{0: nil})
		require.EqualError(t, err, `transaction executor must have non-zero transaction type`)
		require.Empty(t, dst)

		err = dst.Add(TxExecutors{23: nil})
		require.EqualError(t, err, `transaction executor must not be nil (type=23)`)
		require.Empty(t, dst)
	})

	t.Run("adding item with the same name", func(t *testing.T) {
		dst := make(TxExecutors)
		mock := NewMockTxModule(nil)

		require.NoError(t, dst.Add(mock.TxHandlers()))
		require.EqualError(t, dst.Add(mock.TxHandlers()), `transaction executor for type=22 is already registered`)
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
