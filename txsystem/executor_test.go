package txsystem

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/types"
)

func Test_TxExecutors_Execute(t *testing.T) {
	t.Run("unknown tx type", func(t *testing.T) {
		exec := make(TxExecutors)
		require.NoError(t, exec.Add(TxExecutors{"foo": func(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
			return nil, errors.New("unexpected call")
		}}))

		sm, err := exec.Execute(&types.TransactionOrder{Payload: &types.Payload{Type: "bar"}}, &TxExecutionContext{CurrentBlockNr: 5})
		require.Nil(t, sm)
		require.EqualError(t, err, `unknown transaction type bar`)
	})

	t.Run("tx handler returns error", func(t *testing.T) {
		expErr := errors.New("tx handler failed")
		exec := make(TxExecutors)
		require.NoError(t, exec.Add(TxExecutors{"foo": func(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
			require.EqualValues(t, 5, exeCtx.CurrentBlockNr)
			return nil, expErr
		}}))

		sm, err := exec.Execute(&types.TransactionOrder{Payload: &types.Payload{Type: "foo"}}, &TxExecutionContext{CurrentBlockNr: 5})
		require.Nil(t, sm)
		require.ErrorIs(t, err, expErr)
	})

	t.Run("success", func(t *testing.T) {
		exec := make(TxExecutors)
		require.NoError(t, exec.Add(TxExecutors{"foo": func(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
			require.EqualValues(t, 12, exeCtx.CurrentBlockNr)
			return &types.ServerMetadata{}, nil
		}}))

		sm, err := exec.Execute(&types.TransactionOrder{Payload: &types.Payload{Type: "foo"}}, &TxExecutionContext{CurrentBlockNr: 12})
		require.NoError(t, err)
		require.NotNil(t, sm)
	})
}

func Test_TxExecutors_Add(t *testing.T) {
	txHandler := func(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
		return nil, nil
	}

	t.Run("emty inputs", func(t *testing.T) {
		dst := make(TxExecutors)
		// both source and destinations are empty
		require.NoError(t, dst.Add(nil))
		require.Empty(t, dst)

		require.NoError(t, dst.Add(make(TxExecutors)))
		require.Empty(t, dst)

		// when destination is not empty adding empty source to it mustn't change it
		dst["foo"] = txHandler
		require.NoError(t, dst.Add(make(TxExecutors)))
		require.Len(t, dst, 1)
		require.Contains(t, dst, "foo")
	})

	t.Run("attempt to add invalid items", func(t *testing.T) {
		dst := make(TxExecutors)

		err := dst.Add(TxExecutors{"": txHandler})
		require.EqualError(t, err, `tx executor must have non-empty tx type name`)
		require.Empty(t, dst)

		err = dst.Add(TxExecutors{"foo": nil})
		require.EqualError(t, err, `tx executor must not be nil (foo)`)
		require.Empty(t, dst)
	})

	t.Run("adding item with the same name", func(t *testing.T) {
		dst := make(TxExecutors)
		require.NoError(t, dst.Add(TxExecutors{"foo": txHandler}))

		err := dst.Add(TxExecutors{"foo": txHandler})
		require.EqualError(t, err, `tx executor for "foo" is already registered`)
		require.Len(t, dst, 1)
		require.Contains(t, dst, "foo")
	})

	t.Run("success", func(t *testing.T) {
		dst := make(TxExecutors)
		require.NoError(t, dst.Add(TxExecutors{"foo": txHandler}))
		require.NoError(t, dst.Add(TxExecutors{"bar": txHandler, "zoo": txHandler}))
		require.Len(t, dst, 3)
		require.Contains(t, dst, "foo")
		require.Contains(t, dst, "bar")
		require.Contains(t, dst, "zoo")
	})
}
