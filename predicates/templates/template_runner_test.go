package templates

import (
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestTemplateRunner(t *testing.T) {
	t.Parallel()

	runner := New()
	require.EqualValues(t, TemplateStartByte, runner.ID())

	/* invalid inputs */

	t.Run("nil predicate", func(t *testing.T) {
		// we expect that predicates are executed through "dispatcher" which only
		// calls engine with non-nil predicate so engine will not check for that
		// and thus panics with access violation
		require.Panics(t, func() {
			runner.Execute(context.Background(), nil, nil, nil, nil)
		})
	})

	t.Run("invalid predicate tag", func(t *testing.T) {
		res, err := runner.Execute(context.Background(), &predicates.Predicate{Tag: 0x01, Code: []byte{AlwaysFalseID}}, nil, nil, nil)
		require.EqualError(t, err, "expected predicate template tag 0 but got 1")
		require.False(t, res)
	})

	t.Run("predicate code length not 1", func(t *testing.T) {
		res, err := runner.Execute(context.Background(), &predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysFalseID, AlwaysTrueID}}, nil, nil, nil)
		require.EqualError(t, err, "expected predicate template code length to be 1, got 2")
		require.False(t, res)
	})

	t.Run("unknown predicate template", func(t *testing.T) {
		res, err := runner.Execute(context.Background(), &predicates.Predicate{Tag: TemplateStartByte, Code: []byte{0xAF}}, nil, nil, nil)
		require.EqualError(t, err, "unknown predicate template with id 175")
		require.False(t, res)
	})

	/*
		routing to the correct template executor happens (usually the happy case,
		failures of the executor should be tested by executor specific tests)
	*/

	t.Run("always false", func(t *testing.T) {
		af := &predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysFalseID}}
		res, err := runner.Execute(context.Background(), af, nil, nil, nil)
		require.NoError(t, err)
		require.False(t, res)
	})

	t.Run("always true", func(t *testing.T) {
		at := &predicates.Predicate{Tag: TemplateStartByte, Code: []byte{AlwaysTrueID}}
		res, err := runner.Execute(context.Background(), at, nil, nil, nil)
		require.NoError(t, err)
		require.True(t, res)
	})

	t.Run("p2pkh", func(t *testing.T) {
		// easier to check for known error here
		expErr := errors.New("attempt to extract payload bytes")
		execEnv := &mockTxContext{
			payloadBytes: func(txo *types.TransactionOrder) ([]byte, error) { return nil, expErr },
		}
		pred := &predicates.Predicate{Tag: TemplateStartByte, Code: []byte{P2pkh256ID}}
		res, err := runner.Execute(context.Background(), pred, nil, &types.TransactionOrder{}, execEnv)
		require.ErrorIs(t, err, expErr)
		require.False(t, res)
	})
}

type mockTxContext struct {
	getUnit      func(id types.UnitID, committed bool) (*state.Unit, error)
	payloadBytes func(txo *types.TransactionOrder) ([]byte, error)
}

func (env *mockTxContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return env.getUnit(id, committed)
}
func (env *mockTxContext) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	return env.payloadBytes(txo)
}
