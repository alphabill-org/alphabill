package txsystem

import (
	"errors"
	"io"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
)

func Test_NewGenericTxSystem(t *testing.T) {
	t.Run("system ID param is mandatory", func(t *testing.T) {
		txSys, err := NewGenericTxSystem(0, nil, nil, nil, nil)
		require.Nil(t, txSys)
		require.EqualError(t, err, `system ID must be assigned`)
	})

	t.Run("success", func(t *testing.T) {
		obs := observability.Default(t)
		feeCheck := func(env *TxExecutionContext, tx *types.TransactionOrder) error { return errors.New("FCC") }
		txSys, err := NewGenericTxSystem(
			1,
			feeCheck,
			nil,
			nil,
			obs,
		)
		require.NoError(t, err)
		require.EqualValues(t, 1, txSys.systemIdentifier)
		require.NotNil(t, txSys.log)
		require.NotNil(t, txSys.checkFeeCreditBalance)
		require.EqualError(t, txSys.checkFeeCreditBalance(nil, nil), "FCC")
	})
}

func Test_GenericTxSystem_Execute(t *testing.T) {

	createTxSystem := func(t *testing.T, modules []Module) *GenericTxSystem {
		txs, err := NewGenericTxSystem(
			1,
			func(env *TxExecutionContext, tx *types.TransactionOrder) error { return nil }, // "all OK" fee credit validator
			nil,
			modules,
			observability.Default(t),
		)
		require.NoError(t, err)
		txs.currentBlockNumber = 837644
		return txs
	}

	// create valid order (in the sense of basic checks performed by the generic
	// tx system) for "txs" transaction system
	createTxOrder := func(txs *GenericTxSystem) *types.TransactionOrder {
		return &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: txs.systemIdentifier,
				Type:     "tx-type",
				ClientMetadata: &types.ClientMetadata{
					Timeout: txs.currentBlockNumber + 1,
				},
			},
		}
	}

	t.Run("tx order is validated", func(t *testing.T) {
		txSys := createTxSystem(t, nil)
		txo := createTxOrder(txSys)
		// make a change that should cause tx validation fo fail, ie we check that
		// before executing the tx the validateGenericTransaction method is called
		txo.Payload.SystemID = txSys.systemIdentifier + 1
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, ErrInvalidSystemIdentifier)
		require.Nil(t, md)
	})

	t.Run("no executor for the tx type", func(t *testing.T) {
		txSys := createTxSystem(t, nil) // no modules, no tx handlers
		txo := createTxOrder(txSys)
		md, err := txSys.Execute(txo)
		require.EqualError(t, err, `unknown transaction type tx-type`)
		require.Nil(t, md)
	})

	t.Run("tx handler returns error", func(t *testing.T) {
		expErr := errors.New("nope!")
		m := mockModule{
			executors: map[string]ExecuteFunc{
				"tx-type": func(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
					return nil, expErr
				},
			},
		}
		txSys := createTxSystem(t, []Module{m})
		txo := createTxOrder(txSys)
		md, err := txSys.Execute(txo)
		require.ErrorIs(t, err, expErr)
		require.Nil(t, md)
	})

	t.Run("success", func(t *testing.T) {
		m := mockModule{executors: map[string]ExecuteFunc{
			"tx-type": func(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
				return &types.ServerMetadata{SuccessIndicator: types.TxStatusSuccessful}, nil
			},
		},
		}
		txSys := createTxSystem(t, []Module{m})
		txo := createTxOrder(txSys)
		md, err := txSys.Execute(txo)
		require.NoError(t, err)
		require.NotNil(t, md)
	})
}

func Test_GenericTxSystem_validateGenericTransaction(t *testing.T) {

	// share observability between all sub-tests
	obs := observability.Default(t)

	createTxSystem := func(t *testing.T) *GenericTxSystem {
		txs, err := NewGenericTxSystem(
			1,
			func(env *TxExecutionContext, tx *types.TransactionOrder) error { return nil }, // "all OK" fee credit validator
			nil,
			nil, // test doesn't depend on modules
			obs,
		)
		require.NoError(t, err)
		txs.currentBlockNumber = 837644
		return txs
	}

	// create valid order (in the sense of basic checks performed by the generic
	// tx system) for "txs" transaction system
	createTxOrder := func(txs *GenericTxSystem) *types.TransactionOrder {
		return &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID: txs.systemIdentifier,
				ClientMetadata: &types.ClientMetadata{
					Timeout: txs.currentBlockNumber + 1,
				},
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		// this (also) tests that our helper functions do create valid
		// tx system and tx order combination (other tests depend on that)
		txSys := createTxSystem(t)
		txo := createTxOrder(txSys)
		require.NoError(t, txSys.validateGenericTransaction(nil, txo))
	})

	t.Run("system ID is checked", func(t *testing.T) {
		txSys := createTxSystem(t)
		txo := createTxOrder(txSys)
		txo.Payload.SystemID = txSys.systemIdentifier + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(nil, txo), ErrInvalidSystemIdentifier)
	})

	t.Run("timeout is checked", func(t *testing.T) {
		txSys := createTxSystem(t)
		txo := createTxOrder(txSys)

		txSys.currentBlockNumber = txo.Timeout()
		require.ErrorIs(t, txSys.validateGenericTransaction(nil, txo), ErrTransactionExpired)
		txSys.currentBlockNumber = txo.Timeout() + 1
		require.ErrorIs(t, txSys.validateGenericTransaction(nil, txo), ErrTransactionExpired)
		txSys.currentBlockNumber = math.MaxUint64
		require.ErrorIs(t, txSys.validateGenericTransaction(nil, txo), ErrTransactionExpired)
	})

	t.Run("fee credit balance is checked", func(t *testing.T) {
		expErr := errors.New("nope!")
		txSys := createTxSystem(t)
		txSys.checkFeeCreditBalance = func(env *TxExecutionContext, tx *types.TransactionOrder) error { return expErr }
		txo := createTxOrder(txSys)
		require.ErrorIs(t, txSys.validateGenericTransaction(nil, txo), expErr)
	})
}

type mockModule struct {
	executors TxExecutors
}

func (mm mockModule) TxExecutors() map[string]ExecuteFunc {
	return mm.executors
}

type mockUnitState struct {
	ApplyFunc               func(actions ...state.Action) error
	IsCommittedFunc         func() bool
	CalculateRootFunc       func() (uint64, []byte, error)
	PruneFunc               func() error
	GetUnitFunc             func(id types.UnitID, committed bool) (*state.Unit, error)
	AddUnitLogFunc          func(id types.UnitID, transactionRecordHash []byte) error
	CloneFunc               func() *state.State
	CommitFunc              func(uc *types.UnicityCertificate) error
	CommittedUCFunc         func() *types.UnicityCertificate
	SerializeFunc           func(writer io.Writer, committed bool) error
	TraverseFunc            func(traverser avl.Traverser[types.UnitID, *state.Unit])
	RevertFunc              func()
	RollbackToSavepointFunc func(int)
	ReleaseToSavepointFunc  func(int)
	SavepointFunc           func() int
}

func (m *mockUnitState) Apply(actions ...state.Action) error {
	if m.ApplyFunc != nil {
		return m.ApplyFunc(actions...)
	}
	return nil
}

func (m *mockUnitState) IsCommitted() bool {
	if m.IsCommittedFunc != nil {
		return m.IsCommittedFunc()
	}
	return false
}

func (m *mockUnitState) CalculateRoot() (uint64, []byte, error) {
	if m.CalculateRootFunc != nil {
		return m.CalculateRootFunc()
	}
	return 0, nil, nil
}

func (m *mockUnitState) Prune() error {
	if m.PruneFunc != nil {
		return m.PruneFunc()
	}
	return nil
}

func (m *mockUnitState) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	if m.GetUnitFunc != nil {
		return m.GetUnitFunc(id, committed)
	}
	return nil, nil
}

func (m *mockUnitState) AddUnitLog(id types.UnitID, transactionRecordHash []byte) error {
	if m.AddUnitLogFunc != nil {
		return m.AddUnitLogFunc(id, transactionRecordHash)
	}
	return nil
}

func (m *mockUnitState) Clone() *state.State {
	if m.CloneFunc != nil {
		return m.CloneFunc()
	}
	return nil
}

func (m *mockUnitState) Commit(uc *types.UnicityCertificate) error {
	if m.CommitFunc != nil {
		return m.CommitFunc(uc)
	}
	return nil
}

func (m *mockUnitState) CommittedUC() *types.UnicityCertificate {
	if m.CommittedUCFunc != nil {
		return m.CommittedUCFunc()
	}
	return nil
}

func (m *mockUnitState) Serialize(writer io.Writer, committed bool) error {
	if m.SerializeFunc != nil {
		return m.SerializeFunc(writer, committed)
	}
	return nil
}

func (m *mockUnitState) Traverse(traverser avl.Traverser[types.UnitID, *state.Unit]) {
	if m.TraverseFunc != nil {
		m.TraverseFunc(traverser)
	}
}

func (m *mockUnitState) Revert() {
	if m.RevertFunc != nil {
		m.RevertFunc()
	}
}

func (m *mockUnitState) RollbackToSavepoint(id int) {
	if m.RollbackToSavepointFunc != nil {
		m.RollbackToSavepointFunc(id)
	}
}

func (m *mockUnitState) ReleaseToSavepoint(id int) {
	if m.ReleaseToSavepointFunc != nil {
		m.ReleaseToSavepointFunc(id)
	}
}

func (m *mockUnitState) Savepoint() int {
	if m.SavepointFunc != nil {
		return m.SavepointFunc()
	}
	return 0
}
