package txsystem

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
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
