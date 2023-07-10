package avl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpdate_UpdateLeftChild(t *testing.T) {
	tree := initTreeAndAddValues(t)
	require.NoError(t, tree.Update(2, newIntValue(20)))
	require.False(t, tree.root.clean)
	require.True(t, tree.root.right.clean)
	require.False(t, tree.root.left.clean)
	require.Equal(t, int64(20), tree.root.left.value.value)
	require.True(t, tree.root.left.left.clean)

	require.NoError(t, tree.Commit())
	require.Equal(t, int64(33), tree.Root().value.total)
}

func TestUpdate_UpdateRightChild(t *testing.T) {
	tree := initTreeAndAddValues(t)
	require.NoError(t, tree.Update(4, newIntValue(20)))
	require.False(t, tree.root.clean)
	require.False(t, tree.root.right.clean)
	require.True(t, tree.root.left.clean)
	require.Equal(t, int64(20), tree.root.right.value.value)

	require.True(t, tree.root.right.right.clean)
	require.NoError(t, tree.Commit())
	require.Equal(t, int64(31), tree.root.value.total)
}

func TestUpdate_KeyDoesNoeExist_RightSubtree(t *testing.T) {
	tree := initTreeAndAddValues(t)
	require.ErrorContains(t, tree.Update(20, newIntValue(20)), "key 20 does not exist")
	require.True(t, tree.root.clean)
}

func TestUpdate_KeyDoesNoeExist_LeftSubtree(t *testing.T) {
	tree := initTreeAndAddValues(t)
	require.ErrorContains(t, tree.Update(-1, newIntValue(20)), "key -1 does not exist")
	require.True(t, tree.root.clean)
}

func TestUpdate_TreeIsEmpty(t *testing.T) {
	require.ErrorContains(t, newIntTree().Update(20, newIntValue(20)), "key 20 does not exist")
}

func initTreeAndAddValues(t *testing.T) *Tree[IntKey, *Int64Value] {
	tree := newIntTree()
	require.NoError(t, tree.Add(3, newIntValue(3)))
	require.NoError(t, tree.Add(2, newIntValue(2)))
	require.NoError(t, tree.Add(4, newIntValue(4)))
	require.NoError(t, tree.Add(5, newIntValue(5)))
	require.NoError(t, tree.Add(1, newIntValue(1)))
	require.NoError(t, tree.Commit())
	return tree
}
