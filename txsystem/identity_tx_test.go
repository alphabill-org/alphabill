package txsystem

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func Test_IdentityModule(t *testing.T) {
	t.Run("NewIdentityModule", func(t *testing.T) {
		m := NewIdentityModule(nil)
		require.NotNil(t, m)
		require.NotNil(t, m.pr)
	})
}

func Test_IdentityModule_validateIdentityTx(t *testing.T) {
	t.Run("missing payload", func(t *testing.T) {
		m := NewIdentityModule(nil)
		err := m.validateIdentityTx(&types.TransactionOrder{}, nil)
		require.EqualError(t, err, "missing payload")
	})

	t.Run("invalid tx type", func(t *testing.T) {
		m := NewIdentityModule(nil)
		err := m.validateIdentityTx(&types.TransactionOrder{Payload: &types.Payload{Type: "foo"}}, nil)
		require.EqualError(t, err, "invalid tx type: foo")
	})

	t.Run("state lock released, tx does nothing", func(t *testing.T) {
		mockedState := &mockUnitState{
			GetUnitFunc: func(id types.UnitID, committed bool) (*state.Unit, error) {
				return state.NewUnit(templates.AlwaysTrueBytes(), nil), nil
			},
		}
		m := NewIdentityModule(mockedState)
		err := m.validateIdentityTx(&types.TransactionOrder{Payload: &types.Payload{Type: TxIdentity}}, &TxExecutionContext{StateLockReleased: true})
		require.NoError(t, err)
	})

	t.Run("bearer not validated", func(t *testing.T) {
		mockedState := &mockUnitState{
			GetUnitFunc: func(id types.UnitID, committed bool) (*state.Unit, error) {
				return state.NewUnit(templates.AlwaysFalseBytes(), nil), nil
			},
		}
		m := NewIdentityModule(mockedState)
		err := m.validateIdentityTx(&types.TransactionOrder{Payload: &types.Payload{Type: TxIdentity}}, &TxExecutionContext{StateLockReleased: false})
		require.ErrorContains(t, err, "invalid owner proof")
	})

	t.Run("id tx locks state via SetStateLock", func(t *testing.T) {
		mockedState := &mockUnitState{
			GetUnitFunc: func(id types.UnitID, committed bool) (*state.Unit, error) {
				return state.NewUnit(templates.AlwaysTrueBytes(), nil), nil
			},
			ApplyFunc: func(actions ...state.Action) error {
				require.Len(t, actions, 1)
				// actions[0] is a function, check its name via reflection
				funcName := runtime.FuncForPC(reflect.ValueOf(actions[0]).Pointer()).Name()
				require.Contains(t, funcName, "state.SetStateLock")
				return nil
			},
		}
		m := NewIdentityModule(mockedState)
		err := m.validateIdentityTx(&types.TransactionOrder{
			Payload: &types.Payload{
				Type: TxIdentity,
				StateLock: &types.StateLock{
					ExecutionPredicate: templates.AlwaysFalseBytes(),
					RollbackPredicate:  nil,
				},
			},
		}, &TxExecutionContext{StateLockReleased: false})
		require.NoError(t, err)
	})
}
