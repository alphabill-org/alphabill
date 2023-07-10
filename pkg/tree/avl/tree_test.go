package avl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTree_Clone(t *testing.T) {
	tree := initTreeAndAddValues(t)
	tree2 := tree.Clone()
	require.Equal(t, tree.root, tree2.root)
	require.Equal(t, tree.traverser, tree2.traverser)
	require.NoError(t, tree2.Update(2, newIntValue(10)))
	require.NoError(t, tree2.Delete(5))
	require.NoError(t, tree.Commit())
	require.NotEqual(t, tree.root, tree2.root)
	require.Equal(t, *tree.root.left.left, *tree2.root.left.left)
	require.NotEqual(t, *tree.root, *tree2.root)
}

func TestTree_CommitVisitsAllNonCleanNodes(t *testing.T) {
	tree := New[IntKey, *Int64Value]()
	for _, key := range []int{2, 1, 3} {
		require.NoError(t, tree.Add(IntKey(key), &Int64Value{value: int64(key)}))
	}
	require.False(t, tree.root.clean)
	require.False(t, tree.root.left.clean)
	require.False(t, tree.root.right.clean)
	require.NoError(t, tree.Commit())
	require.True(t, tree.root.clean)
	require.True(t, tree.root.left.clean)
	require.True(t, tree.root.right.clean)
}
