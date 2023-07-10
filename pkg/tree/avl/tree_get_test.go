package avl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGet(t *testing.T) {
	tree := initTreeAndAddValues(t)
	for _, i := range []IntKey{3, 2, 4, 5, 1} {
		v, err := tree.Get(i)
		require.NoError(t, err)
		require.Equal(t, int64(i), v.value)
	}
}

func TestGet_TreeEmpty(t *testing.T) {
	tree := New[IntKey, *Int64Value]()
	_, err := tree.Get(100)
	require.ErrorContains(t, err, "not found")
}

func TestGet_NotFoundEmpty(t *testing.T) {
	tree := initTreeAndAddValues(t)
	_, err := tree.Get(100)
	require.ErrorContains(t, err, "not found")
}
