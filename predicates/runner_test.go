package predicates

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func Test_Dispatcher(t *testing.T) {
	// no parameters
	eng, err := Dispatcher()
	require.NoError(t, err)
	require.NotNil(t, eng)
	require.Empty(t, eng)

	// single engine as argument
	eng, err = Dispatcher(mockPredicateEngine{id: 3})
	require.NoError(t, err)
	require.Len(t, eng, 1)
	require.Contains(t, eng, uint64(3))

	// two engines
	eng, err = Dispatcher(mockPredicateEngine{id: 3}, mockPredicateEngine{id: 2})
	require.NoError(t, err)
	require.Len(t, eng, 2)
	require.Contains(t, eng, uint64(2))
	require.Contains(t, eng, uint64(3))

	// same engine twice
	eng, err = Dispatcher(mockPredicateEngine{id: 5}, mockPredicateEngine{id: 5})
	require.EqualError(t, err, `registering predicate engine 2 of 2: predicate engine with id 5 is already registered`)
	require.Len(t, eng, 0)
}

func Test_PredicateEngines_Add(t *testing.T) {
	eng, err := Dispatcher()
	require.NoError(t, err)

	require.NoError(t, eng.Add(mockPredicateEngine{id: 0}))
	require.Len(t, eng, 1)
	require.Contains(t, eng, uint64(0))

	// attempt to add engine with the same id, should fail but not alter the content of engine collection
	require.EqualError(t, eng.Add(mockPredicateEngine{id: 0}), `predicate engine with id 0 is already registered`)
	require.Len(t, eng, 1)
	require.Contains(t, eng, uint64(0))

	// should be able to add engine with different id
	require.NoError(t, eng.Add(mockPredicateEngine{id: 1}))
	require.Len(t, eng, 2)
	require.Contains(t, eng, uint64(0))
	require.Contains(t, eng, uint64(1))
}

func Test_PredicateEngines_Execute(t *testing.T) {
	t.Run("predicate binary size", func(t *testing.T) {
		// must fail before dispatching to engine so OK not to have any
		eng, err := Dispatcher()
		require.NoError(t, err)

		// nil
		res, err := eng.Execute(context.Background(), nil, nil, nil, nil)
		require.EqualError(t, err, `predicate is empty`)
		require.False(t, res)
		// empty slice
		res, err = eng.Execute(context.Background(), []byte{}, nil, nil, nil)
		require.EqualError(t, err, `predicate is empty`)
		require.False(t, res)
		// too big
		res, err = eng.Execute(context.Background(), make([]byte, MaxPredicateBinSize+1), nil, nil, nil)
		require.EqualError(t, err, fmt.Sprintf("predicate is too large, max allowed is %d got %d bytes", MaxPredicateBinSize, MaxPredicateBinSize+1))
		require.False(t, res)
	})

	t.Run("invalid predicate CBOR", func(t *testing.T) {
		eng, err := Dispatcher()
		require.NoError(t, err)

		res, err := eng.Execute(context.Background(), []byte("this ain't valid predicate"), nil, nil, nil)
		require.EqualError(t, err, `decoding predicate: cbor: 5 bytes of extraneous data starting at index 21`)
		require.False(t, res)
	})

	t.Run("predicate for unknown engine", func(t *testing.T) {
		eng, err := Dispatcher(mockPredicateEngine{id: 1})
		require.NoError(t, err)

		pred := predicates.Predicate{Tag: 3}
		bin, err := pred.AsBytes()
		require.NoError(t, err)

		res, err := eng.Execute(context.Background(), bin, nil, nil, nil)
		require.EqualError(t, err, `unknown predicate engine with id 3`)
		require.False(t, res)
	})

	t.Run("executor returns error", func(t *testing.T) {
		expErr := errors.New("can't eval this predicate")
		eng, err := Dispatcher(
			mockPredicateEngine{
				id: 1,
				exec: func(ctx context.Context, predicate *predicates.Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
					return false, expErr
				},
			},
		)
		require.NoError(t, err)

		pred := predicates.Predicate{Tag: 1}
		bin, err := pred.AsBytes()
		require.NoError(t, err)

		res, err := eng.Execute(context.Background(), bin, nil, nil, nil)
		require.ErrorIs(t, err, expErr)
		require.False(t, res)
	})

	t.Run("executor returns false", func(t *testing.T) {
		eng, err := Dispatcher(
			mockPredicateEngine{
				id: 1,
				exec: func(ctx context.Context, predicate *predicates.Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
					return false, nil
				},
			},
		)
		require.NoError(t, err)

		pred := predicates.Predicate{Tag: 1}
		bin, err := pred.AsBytes()
		require.NoError(t, err)

		res, err := eng.Execute(context.Background(), bin, nil, nil, nil)
		require.NoError(t, err)
		require.False(t, res)
	})

	t.Run("executor returns true", func(t *testing.T) {
		pred := &predicates.Predicate{Tag: 1, Code: []byte{8, 8, 8}, Params: []byte{5, 5, 5}}
		bin, err := pred.AsBytes()
		require.NoError(t, err)

		eng, err := Dispatcher(
			mockPredicateEngine{
				id: 1,
				exec: func(ctx context.Context, predicate *predicates.Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
					require.Equal(t, pred, predicate)
					return true, nil
				},
			},
		)
		require.NoError(t, err)

		res, err := eng.Execute(context.Background(), bin, nil, nil, nil)
		require.NoError(t, err)
		require.True(t, res)
	})
}

func Test_PredicateRunner(t *testing.T) {
	t.Run("executor returns error", func(t *testing.T) {
		expErr := errors.New("evaluation failed")
		exec := NewPredicateRunner(
			func(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
				return false, expErr
			},
		)
		require.NotNil(t, exec)

		err := exec([]byte("predicate"), []byte("arguments"), nil, nil)
		require.ErrorIs(t, err, expErr)
	})

	t.Run("evals to false", func(t *testing.T) {
		exec := NewPredicateRunner(
			func(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
				return false, nil
			},
		)
		require.NotNil(t, exec)

		err := exec([]byte("predicate"), []byte("arguments"), nil, nil)
		require.EqualError(t, err, `predicate evaluated to "false"`)
	})

	t.Run("evals to true", func(t *testing.T) {
		exec := NewPredicateRunner(
			func(ctx context.Context, predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
				require.EqualValues(t, []byte("predicate"), predicate)
				require.EqualValues(t, []byte("arguments"), args)
				return true, nil
			},
		)
		require.NotNil(t, exec)

		err := exec([]byte("predicate"), []byte("arguments"), nil, nil)
		require.NoError(t, err)
	})
}

func Test_Predicate_AsBytes(t *testing.T) {
	pred := &predicates.Predicate{Tag: 42, Code: []byte{8, 8, 8}, Params: []byte{5, 5, 5}}
	bin, err := pred.AsBytes()
	require.NoError(t, err)

	p2, err := ExtractPredicate(bin)
	require.NoError(t, err)
	require.Equal(t, pred, p2)
}

type mockPredicateEngine struct {
	id   uint64
	exec func(ctx context.Context, predicate *predicates.Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error)
}

func (pe mockPredicateEngine) ID() uint64 { return pe.id }

func (pe mockPredicateEngine) Execute(ctx context.Context, predicate *predicates.Predicate, args []byte, txo *types.TransactionOrder, env TxContext) (bool, error) {
	return pe.exec(ctx, predicate, args, txo, env)
}
