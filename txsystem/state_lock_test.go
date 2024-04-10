package txsystem

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_StateUnlockProofFromBytes(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		_, err := StateUnlockProofFromBytes([]byte{})
		require.Error(t, err)
		require.Equal(t, "invalid state unlock proof: empty", err.Error())
	})

	t.Run("valid input execute kind", func(t *testing.T) {
		kind := StateUnlockExecute
		proof := []byte("proof")
		input := append([]byte{byte(kind)}, proof...)

		result, err := StateUnlockProofFromBytes(input)
		require.NoError(t, err)
		require.Equal(t, kind, result.Kind)
		require.Equal(t, proof, result.Proof)
	})

	t.Run("valid input rollback kind", func(t *testing.T) {
		kind := StateUnlockRollback
		proof := []byte("proof")
		input := append([]byte{byte(kind)}, proof...)

		result, err := StateUnlockProofFromBytes(input)
		require.NoError(t, err)
		require.Equal(t, kind, result.Kind)
		require.Equal(t, proof, result.Proof)
	})

	t.Run("invalid kind", func(t *testing.T) {
		kind := byte(2) // Invalid kind
		proof := []byte("proof")
		input := append([]byte{kind}, proof...)

		result, err := StateUnlockProofFromBytes(input)
		require.NoError(t, err)
		require.NotEqual(t, StateUnlockExecute, result.Kind)
		require.NotEqual(t, StateUnlockRollback, result.Kind)
		require.Equal(t, proof, result.Proof)
	})
}
