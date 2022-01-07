package tree

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	key1  = uint256.NewInt(1)
	key2  = uint256.NewInt(2)
	key3  = uint256.NewInt(3)
	key4  = uint256.NewInt(4)
	key5  = uint256.NewInt(5)
	key6  = uint256.NewInt(6)
	key7  = uint256.NewInt(7)
	key8  = uint256.NewInt(8)
	key9  = uint256.NewInt(9)
	key10 = uint256.NewInt(10)
	key11 = uint256.NewInt(11)
	key12 = uint256.NewInt(12)
	key13 = uint256.NewInt(13)
	key15 = uint256.NewInt(15)
	key20 = uint256.NewInt(20)
	key24 = uint256.NewInt(24)
	key25 = uint256.NewInt(25)
	key30 = uint256.NewInt(30)
	key31 = uint256.NewInt(31)
)

func TestPut_Owerwrite(t *testing.T) {
	var root *Node
	put(key1, newNodeContent(1), nil, &root)
	requireNodeEquals(t, root, key1, 1)
	put(key1, newNodeContent(2), nil, &root)
	requireNodeEquals(t, root, key1, 2)
}

func TestAddBill_AVLTreeRotateLeft(t *testing.T) {
	var root *Node

	put(key1, newNodeContent(1), nil, &root)
	put(key2, newNodeContent(2), nil, &root)
	put(key3, newNodeContent(3), nil, &root)

	requireNodeEquals(t, root, key2, 2)
	requireNodeEquals(t, root.Children[0], key1, 1)
	requireNodeEquals(t, root.Children[1], key3, 3)
}

func TestAddBill_AVLTreeRotateRight(t *testing.T) {
	var root *Node

	put(key3, newNodeContent(3), nil, &root)
	put(key2, newNodeContent(2), nil, &root)
	put(key1, newNodeContent(1), nil, &root)

	requireNodeEquals(t, root, key2, 2)
	requireNodeEquals(t, root.Children[0], key1, 1)
	requireNodeEquals(t, root.Children[1], key3, 3)
}

func TestAddBill_AVLTreeRotateLeftRight(t *testing.T) {
	var root *Node

	put(key10, newNodeContent(10), nil, &root)
	put(key20, newNodeContent(20), nil, &root)
	put(key30, newNodeContent(30), nil, &root)
	put(key1, newNodeContent(1), nil, &root)
	put(key15, newNodeContent(15), nil, &root)
	put(key12, newNodeContent(12), nil, &root)

	requireNodeEquals(t, root, key15, 15)
	requireNodeEquals(t, root.Children[0], key10, 10)
	requireNodeEquals(t, root.Children[0].Children[0], key1, 1)
	requireNodeEquals(t, root.Children[0].Children[1], key12, 12)
	requireNodeEquals(t, root.Children[1], key20, 20)
	requireNodeEquals(t, root.Children[1].Children[1], key30, 30)
}

func TestAddBill_AVLTreeRotateRightLeft(t *testing.T) {
	var root *Node

	put(key10, nil, nil, &root)
	put(key30, nil, nil, &root)
	put(key20, nil, nil, &root)

	put(key25, nil, nil, &root)
	put(key31, nil, nil, &root)
	put(key24, nil, nil, &root)

	assert.Equal(t, root.ID, key25)
	assert.Equal(t, root.Children[0].ID, key20)
	assert.Equal(t, root.Children[0].Children[0].ID, key10)
	assert.Equal(t, root.Children[0].Children[1].ID, key24)
	assert.Equal(t, root.Children[1].ID, key30)
	assert.Equal(t, root.Children[1].Children[1].ID, key31)
}

func TestGetNode_LeftChild(t *testing.T) {
	var root *Node

	put(key1, newNodeContent(1), nil, &root)
	put(key2, newNodeContent(2), nil, &root)
	put(key3, newNodeContent(3), nil, &root)

	node, found := getNode(root, key1)
	assert.True(t, found)
	assert.NotNil(t, node)
	requireNodeEquals(t, node, key1, 1)
	assert.Nil(t, node.Children[0])
	assert.Nil(t, node.Children[1])
}

func TestGetNode_RightChild(t *testing.T) {
	var root *Node

	put(key1, nil, nil, &root)
	put(key2, nil, nil, &root)
	put(key3, nil, nil, &root)

	node, found := getNode(root, key3)
	assert.True(t, found)
	assert.NotNil(t, node)
}

func TestGetNode_NotFound(t *testing.T) {
	var root *Node

	put(key1, nil, nil, &root)
	put(key2, nil, nil, &root)
	put(key3, nil, nil, &root)

	node, found := getNode(root, key4)
	assert.False(t, found)
	assert.Nil(t, node)
}

func TestDeleteNode_empty(t *testing.T) {
	var root *Node
	put(key1, nil, nil, &root)
	remove(key1, &root)
	require.Nil(t, root)
}

func TestDeleteNode_NonExisting(t *testing.T) {
	var root *Node
	put(key1, nil, nil, &root)
	remove(key2, &root)
	require.Equal(t, key1, root.ID)
}

func TestRemoveNode_TwoNodes(t *testing.T) {
	var root *Node
	put(key1, newNodeContent(1), nil, &root)
	put(key2, newNodeContent(2), nil, &root)
	remove(key2, &root)
	requireNodeExists(t, root, key1)
	requireNodeMissing(t, root, key2)

	root = nil
	put(key1, newNodeContent(1), nil, &root)
	put(key2, newNodeContent(2), nil, &root)
	remove(key1, &root)
	requireNodeMissing(t, root, key1)
	requireNodeExists(t, root, key2)
}

func TestRemoveNode_Leaf(t *testing.T) {
	var root *Node

	put(key1, newNodeContent(1), nil, &root)
	put(key2, newNodeContent(2), nil, &root)
	put(key3, newNodeContent(3), nil, &root)

	remove(key3, &root)

	requireNodeExists(t, root, key1)
	requireNodeExists(t, root, key2)
	requireNodeMissing(t, root, key3)
}

func TestRemoveNode_Top(t *testing.T) {
	var root *Node

	put(key1, newNodeContent(1), nil, &root)
	put(key2, newNodeContent(2), nil, &root)
	put(key3, newNodeContent(3), nil, &root)

	remove(key2, &root)

	requireNodeExists(t, root, key1)
	requireNodeMissing(t, root, key2)
	requireNodeExists(t, root, key3)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_RightRight(t *testing.T) {
	var root *Node

	for i := 1; i < 11; i++ {
		put(uint256.NewInt(uint64(i)), newNodeContent(i), nil, &root)
	}

	// Make the right-right child node subtree is the highest.
	remove(key5, &root)
	remove(key7, &root)

	//printTree(root)
	// Trigger rotation by deleting left sub node children.
	remove(key3, &root)
	//printTree(root)
	remove(key1, &root)
	//printTree(root)

	// Node 4 became child and 6 was moved under it.
	require.Equal(t, key8, root.ID)
	require.Equal(t, key4, root.Children[0].ID)
	require.Equal(t, key6, root.Children[0].Children[1].ID)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_RightLeft(t *testing.T) {
	var root *Node

	for i := 1; i < 11; i++ {
		put(uint256.NewInt(uint64(i)), newNodeContent(i), nil, &root)
	}

	//printTree(root)
	// Make right-left child node subtree the highest.
	remove(key10, &root)

	//printTree(root)
	// Trigger rotation by deleting left sub node children.
	remove(key1, &root)
	//printTree(root)
	remove(key3, &root)
	//printTree(root)

	// Node 6 becomes the root by two rotations.
	require.Equal(t, key6, root.ID)
	require.Equal(t, key8, root.Children[1].ID)
	require.Equal(t, key4, root.Children[0].ID)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_LeftLeft(t *testing.T) {
	var root *Node

	for i := 1; i < 14; i++ {
		put(uint256.NewInt(uint64(i)), newNodeContent(i), nil, &root)
	}

	//printTree(root)
	// Make left-left child node subtree the highest.
	remove(key11, &root)
	remove(key13, &root)
	remove(key7, &root)
	remove(key5, &root)

	//printTree(root)

	// Trigger balancing by deleting sub nodes from right child.
	remove(key12, &root)
	//printTree(root)
	remove(key9, &root)
	//printTree(root)

	// Node 4 becomes the root by one rotation.
	require.Equal(t, key4, root.ID)
	require.Equal(t, key8, root.Children[1].ID)
	require.Equal(t, key6, root.Children[1].Children[0].ID)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_LeftRight(t *testing.T) {
	var root *Node

	for i := 1; i < 14; i++ {
		put(uint256.NewInt(uint64(i)), newNodeContent(i), nil, &root)
	}

	//printTree(root)
	// Make left-left child node subtree the highest.
	remove(key11, &root)
	remove(key13, &root)
	remove(key3, &root)
	remove(key1, &root)

	//printTree(root)

	// Trigger balancing by deleting sub nodes from right child.
	remove(key12, &root)
	//printTree(root)
	remove(key9, &root)
	//printTree(root)

	// Node 6 was becomes the root by two rotations.
	require.Equal(t, key6, root.ID)
	require.Equal(t, key8, root.Children[1].ID)
	require.Equal(t, key7, root.Children[1].Children[0].ID)
}

func printTree(root *Node) {
	out := ""
	output(root, "", false, &out)
	println(out)
}

func requireNodeEquals(t *testing.T, node *Node, key *uint256.Int, val int) {
	require.Equal(t, key, node.ID)
	value, ok := node.Content.Data.(TestData)
	require.True(t, ok, "should be TestData, as inserted")
	require.Equal(t, TestData(val), value)
	require.Equal(t, Predicate{byte(val)}, node.Content.Bearer)

	require.Equal(t, []byte{byte(val)}, node.Content.StateHash)
}

func newNodeContent(val int) *NodeContent {
	return &NodeContent{
		Bearer:    Predicate{byte(val)},
		Data:      TestData(uint64(val)),
		StateHash: []byte{byte(val)},
	}
}

func requireNodeMissing(t *testing.T, root *Node, key *uint256.Int) {
	actualNode, found := getNode(root, key)
	require.False(t, found, "node %v exists, but should be missing", key)
	require.Nil(t, actualNode)
}

func requireNodeExists(t *testing.T, root *Node, key *uint256.Int) {
	actualNode, found := getNode(root, key)
	require.True(t, found, "node %v is missing, but should exist", key)
	require.NotNil(t, actualNode)
}
