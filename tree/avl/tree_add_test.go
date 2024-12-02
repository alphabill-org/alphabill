package avl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type IntTreeTraverser struct{}

type Int64Value struct {
	value int64
	total int64
}

func newIntValue(value int64) *Int64Value {
	return &Int64Value{value: value, total: value}
}

func (i *Int64Value) Clone() *Int64Value {
	return &Int64Value{
		value: i.value,
		total: i.total,
	}
}

func (i *Int64Value) String() string {
	return fmt.Sprintf("value=%d, total=%d", i.value, i.total)
}

func TestAdd_KeyExists_RightChild(t *testing.T) {
	tree := newIntTree()
	require.NoError(t, tree.Add(2, newIntValue(2)))
	require.NoError(t, tree.Add(1, newIntValue(1)))
	require.NoError(t, tree.Add(4, newIntValue(4)))
	require.NoError(t, tree.Add(3, newIntValue(3)))
	require.NoError(t, tree.Add(5, newIntValue(5)))
	tree.Commit()

	require.ErrorContains(t, tree.Add(4, newIntValue(8)), "key 4 exists")
	require.True(t, tree.root.clean)
	require.True(t, tree.root.right.clean)
	require.Equal(t, int64(4), tree.root.right.value.value)
	require.Equal(t, int64(15), tree.root.value.total)
}

func (*IntTreeTraverser) Traverse(n *Node[IntKey, *Int64Value]) error {
	if n == nil || n.clean {
		return nil
	}
	sum(n)
	return nil
}

func TestAdd_KeyExists_LeftChild(t *testing.T) {
	tree := newIntTree()
	require.NoError(t, tree.Add(3, newIntValue(3)))
	require.NoError(t, tree.Add(2, newIntValue(2)))
	require.NoError(t, tree.Add(4, newIntValue(4)))
	require.NoError(t, tree.Add(5, newIntValue(5)))
	require.NoError(t, tree.Add(1, newIntValue(1)))
	tree.Commit()

	require.ErrorContains(t, tree.Add(2, newIntValue(8)), "key 2 exists")
	require.True(t, tree.root.clean)
	require.True(t, tree.root.left.clean)
	require.Equal(t, int64(2), tree.root.left.value.value)
	require.Equal(t, int64(15), tree.root.value.total)
}

func TestAdd_Rotations(t *testing.T) {
	tests := []struct {
		name          string
		prepareValues []value
		valueToAdd    value
		expectedTotal int64
		expectedOrder []order // in post-order
	}{
		{
			name: "rotate left",
			prepareValues: []value{
				{key: 10, value: 10},
				{key: 20, value: 20},
			},
			valueToAdd:    value{30, 30},
			expectedTotal: 60,
			expectedOrder: []order{
				{value: value{key: 10, value: 10}, clean: false, depth: 1},
				{value: value{key: 30, value: 30}, clean: false, depth: 1},
				{value: value{key: 20, value: 20}, clean: false, depth: 2},
			},
		},
		{
			name: "rotate right",
			prepareValues: []value{
				{key: 20, value: 20},
				{key: 10, value: 10},
			},
			valueToAdd:    value{5, 5},
			expectedTotal: 35,
			expectedOrder: []order{
				{value: value{key: 5, value: 5}, clean: false, depth: 1},
				{value: value{key: 20, value: 20}, clean: false, depth: 1},
				{value: value{key: 10, value: 10}, clean: false, depth: 2},
			},
		},
		{
			name: "rotate left right",
			prepareValues: []value{
				//		┌───┤ key=30, depth=1, value=30
				//	────┤ key=20, depth=3, value=20
				//		│	┌───┤ key=15, depth=1, value=1
				//		└───┤ key=10, depth=2, value=10
				//			└───┤ key=1, depth=1, value=10
				{key: 20, value: 20},
				{key: 10, value: 10},
				{key: 30, value: 30},
				{key: 1, value: 1},
				{key: 15, value: 15},
			},
			valueToAdd:    value{12, 12},
			expectedTotal: 88,
			expectedOrder: []order{
				//			┌───┤ key=30, depth=1, value=30
				//		┌───┤ key=20, depth=2, value=20
				//	────┤ key=15, depth=3, value=15
				//		│	┌───┤ key=12, depth=1, value=12
				//		└───┤ key=10, depth=2, value=10
				//			└───┤ key=1, depth=1, value=1
				{value: value{key: 1, value: 1}, clean: true, depth: 1},
				{value: value{key: 12, value: 12}, clean: false, depth: 1},
				{value: value{key: 10, value: 10}, clean: false, depth: 2},
				{value: value{key: 30, value: 30}, clean: true, depth: 1},
				{value: value{key: 20, value: 20}, clean: false, depth: 2},
				{value: value{key: 15, value: 15}, clean: false, depth: 3},
			},
		},
		{
			name: "rotate right left",
			prepareValues: []value{
				//			┌───┤ key=31, depth=1, value=31
				//		┌───┤ key=30, depth=2, value=30
				//		│	└───┤ key=25, depth=1, value=25
				//	────┤ key=20, depth=3, value=20
				//		└───┤ key=10, depth=1, value=10
				{key: 20, value: 20},
				{key: 10, value: 10},
				{key: 30, value: 30},
				{key: 25, value: 25},
				{key: 31, value: 31},
			},
			valueToAdd:    value{24, 24},
			expectedTotal: 140,
			expectedOrder: []order{
				//			┌───┤ key=31, depth=1, value=31
				//		┌───┤ key=30, depth=2, value=30
				//	────┤ key=25, depth=3, value=25
				//		│	┌───┤ key=24, depth=1, value=24
				//		└───┤ key=20, depth=2, value=20
				//			└───┤ key=10, depth=1, value=10
				{value: value{key: 10, value: 10}, clean: true, depth: 1},
				{value: value{key: 24, value: 24}, clean: false, depth: 1},
				{value: value{key: 20, value: 20}, clean: false, depth: 2},
				{value: value{key: 31, value: 31}, clean: true, depth: 1},
				{value: value{key: 30, value: 30}, clean: false, depth: 2},
				{value: value{key: 25, value: 25}, clean: false, depth: 3},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree := newIntTree()
			for _, v := range test.prepareValues {
				require.NoError(t, tree.Add(v.key, newIntValue(v.value)))
			}
			tree.Commit()
			require.NoError(t, tree.Add(test.valueToAdd.key, newIntValue(test.valueToAdd.value)))
			postOrder := postOrderNodes(tree.root)
			require.Len(t, postOrder, len(test.expectedOrder))
			for i, o := range test.expectedOrder {
				node := postOrder[i]
				require.Equal(t, o.value.key, node.Key(), "invalid node key: %v, required: %v", node.key, o.value.key)
				require.Equal(t, o.value.value, node.Value().value, "node %d has invalid value: %v, required: %v", node.key, node.value.value, o.value.value)
				require.Equal(t, o.clean, node.clean, "node %d has invalid clean flag: %v, required: %v", node.key, node.clean, o.clean)
				require.Equal(t, o.depth, node.depth, "node %d has invalid depth: %v, required: %v", node.key, node.depth, o.depth)
			}
			tree.Commit()
			require.Equal(t, test.expectedTotal, tree.root.value.total)
		})
	}
}

func newIntTree() *Tree[IntKey, *Int64Value] {
	t := New[IntKey, *Int64Value]()
	t.traverser = &IntTreeTraverser{}
	return t
}

func sum(n *Node[IntKey, *Int64Value]) {
	if n == nil || n.clean {
		return
	}
	sum(n.left)
	sum(n.right)
	lt := int64(0)
	if n.left != nil {
		lt = n.left.value.total
	}
	rt := int64(0)
	if n.right != nil {
		rt = n.right.value.total
	}

	n.value.total = n.value.value + lt + rt
	n.clean = true
}
