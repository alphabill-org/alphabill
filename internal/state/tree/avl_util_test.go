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
	key10 = uint256.NewInt(10)
	key12 = uint256.NewInt(12)
	key15 = uint256.NewInt(15)
	key20 = uint256.NewInt(20)
	key24 = uint256.NewInt(24)
	key25 = uint256.NewInt(25)
	key30 = uint256.NewInt(30)
	key31 = uint256.NewInt(31)
)

func TestPut_Owerwrite(t *testing.T) {
	var root *Node
	put(key1, TestData(1), Predicate{1}, nil, &root)
	requireNodeEquals(t, root, key1, 1)
	put(key1, TestData(2), Predicate{2}, nil, &root)
	requireNodeEquals(t, root, key1, 2)
}

func TestAddBill_AVLTreeRotateLeft(t *testing.T) {
	var root *Node

	put(key1, TestData(1), Predicate{1}, nil, &root)
	put(key2, TestData(2), Predicate{2}, nil, &root)
	put(key3, TestData(3), Predicate{3}, nil, &root)

	requireNodeEquals(t, root, key2, 2)
	requireNodeEquals(t, root.Children[0], key1, 1)
	requireNodeEquals(t, root.Children[1], key3, 3)
}

func TestAddBill_AVLTreeRotateRight(t *testing.T) {
	var root *Node

	put(key3, TestData(3), Predicate{3}, nil, &root)
	put(key2, TestData(2), Predicate{2}, nil, &root)
	put(key1, TestData(1), Predicate{1}, nil, &root)

	requireNodeEquals(t, root, key2, 2)
	requireNodeEquals(t, root.Children[0], key1, 1)
	requireNodeEquals(t, root.Children[1], key3, 3)
}

func TestAddBill_AVLTreeRotateLeftRight(t *testing.T) {
	var root *Node

	put(key10, TestData(10), Predicate{10}, nil, &root)
	put(key20, TestData(20), Predicate{20}, nil, &root)
	put(key30, TestData(30), Predicate{30}, nil, &root)
	put(key1, TestData(1), Predicate{1}, nil, &root)
	put(key15, TestData(15), Predicate{15}, nil, &root)
	put(key12, TestData(12), Predicate{12}, nil, &root)

	requireNodeEquals(t, root, key15, 15)
	requireNodeEquals(t, root.Children[0], key10, 10)
	requireNodeEquals(t, root.Children[0].Children[0], key1, 1)
	requireNodeEquals(t, root.Children[0].Children[1], key12, 12)
	requireNodeEquals(t, root.Children[1], key20, 20)
	requireNodeEquals(t, root.Children[1].Children[1], key30, 30)
}

func TestAddBill_AVLTreeRotateRightLeft(t *testing.T) {
	var root *Node

	put(key10, nil, nil, nil, &root)
	put(key30, nil, nil, nil, &root)
	put(key20, nil, nil, nil, &root)

	put(key25, nil, nil, nil, &root)
	put(key31, nil, nil, nil, &root)
	put(key24, nil, nil, nil, &root)

	assert.Equal(t, root.ID, key25)
	assert.Equal(t, root.Children[0].ID, key20)
	assert.Equal(t, root.Children[0].Children[0].ID, key10)
	assert.Equal(t, root.Children[0].Children[1].ID, key24)
	assert.Equal(t, root.Children[1].ID, key30)
	assert.Equal(t, root.Children[1].Children[1].ID, key31)
}

func TestGetNode_LeftChild(t *testing.T) {
	var root *Node

	put(key1, TestData(1), Predicate{1}, nil, &root)
	put(key2, TestData(2), Predicate{2}, nil, &root)
	put(key3, TestData(3), Predicate{3}, nil, &root)

	node, found := getNode(root, key1)
	assert.True(t, found)
	assert.NotNil(t, node)
	requireNodeEquals(t, node, key1, 1)
	assert.Nil(t, node.Children[0])
	assert.Nil(t, node.Children[1])
}

func TestGetNode_RightChild(t *testing.T) {
	var root *Node

	put(key1, nil, nil, nil, &root)
	put(key2, nil, nil, nil, &root)
	put(key3, nil, nil, nil, &root)

	node, found := getNode(root, key3)
	assert.True(t, found)
	assert.NotNil(t, node)
}

func TestGetNode_NotFound(t *testing.T) {
	var root *Node

	put(key1, nil, nil, nil, &root)
	put(key2, nil, nil, nil, &root)
	put(key3, nil, nil, nil, &root)

	node, found := getNode(root, key4)
	assert.False(t, found)
	assert.Nil(t, node)
}

func requireNodeEquals(t *testing.T, node *Node, key *uint256.Int, val int) {
	require.Equal(t, key, node.ID)
	value, ok := node.Data.(TestData)
	require.True(t, ok, "should be TestData, as inserted")
	require.Equal(t, TestData(val), value)
	require.Equal(t, Predicate{byte(val)}, node.Bearer)
}
