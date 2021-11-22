package state

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	key1  = uint64(1)
	key2  = uint64(2)
	key3  = uint64(3)
	key4  = uint64(4)
	key10 = uint64(10)
	key12 = uint64(12)
	key15 = uint64(15)
	key20 = uint64(20)
	key24 = uint64(24)
	key25 = uint64(25)
	key30 = uint64(30)
	key31 = uint64(31)
)

func TestAddBill_AVLTreeRotateLeft(t *testing.T) {
	var root *Node

	put(key1, newBillContent(1), nil, &root)
	put(key2, newBillContent(2), nil, &root)
	put(key3, newBillContent(3), nil, &root)

	assert.Equal(t, root.ID, key2)
	assert.Equal(t, root.Children[0].ID, key1)
	assert.Equal(t, root.Children[1].ID, key3)
}

func TestAddBill_AVLTreeRotateRight(t *testing.T) {
	var root *Node

	put(key3, newBillContent(3), nil, &root)
	put(key2, newBillContent(2), nil, &root)
	put(key1, newBillContent(1), nil, &root)

	assert.Equal(t, root.ID, key2)
	assert.Equal(t, root.Children[0].ID, key1)
	assert.Equal(t, root.Children[1].ID, key3)
}

func TestAddBill_AVLTreeRotateLeftRight(t *testing.T) {
	var root *Node

	put(key10, newBillContent(10), nil, &root)
	put(key20, newBillContent(20), nil, &root)
	put(key30, newBillContent(30), nil, &root)
	put(key1, newBillContent(1), nil, &root)
	put(key15, newBillContent(15), nil, &root)
	put(key12, newBillContent(12), nil, &root)

	assert.Equal(t, root.ID, key15)
	assert.Equal(t, root.Children[0].ID, key10)
	assert.Equal(t, root.Children[0].Children[0].ID, key1)
	assert.Equal(t, root.Children[0].Children[1].ID, key12)
	assert.Equal(t, root.Children[1].ID, key20)
	assert.Equal(t, root.Children[1].Children[1].ID, key30)
}

func TestAddBill_AVLTreeRotateRightLeft(t *testing.T) {
	var root *Node

	put(key10, newBillContent(10), nil, &root)
	put(key30, newBillContent(30), nil, &root)
	put(key20, newBillContent(20), nil, &root)

	put(key25, newBillContent(25), nil, &root)
	put(key31, newBillContent(31), nil, &root)
	put(key24, newBillContent(24), nil, &root)

	assert.Equal(t, root.ID, key25)
	assert.Equal(t, root.Children[0].ID, key20)
	assert.Equal(t, root.Children[0].Children[0].ID, key10)
	assert.Equal(t, root.Children[0].Children[1].ID, key24)
	assert.Equal(t, root.Children[1].ID, key30)
	assert.Equal(t, root.Children[1].Children[1].ID, key31)
}

func TestGetNode_LeftChild(t *testing.T) {
	var root *Node

	put(key1, newBillContent(1), nil, &root)
	put(key2, newBillContent(2), nil, &root)
	put(key3, newBillContent(3), nil, &root)

	node, found := getNode(root, key1)
	assert.True(t, found)
	assert.NotNil(t, node)
	assert.Equal(t, key1, node.ID)
	assert.Equal(t, uint32(1), node.Bill.Value)
	assert.Nil(t, node.Children[0])
	assert.Nil(t, node.Children[1])
}

func TestGetNode_RightChild(t *testing.T) {
	var root *Node
	
	put(key1, newBillContent(1), nil, &root)
	put(key2, newBillContent(2), nil, &root)
	put(key3, newBillContent(3), nil, &root)

	node, found := getNode(root, key3)
	assert.True(t, found)
	assert.NotNil(t, node)
}

func TestGetNode_NotFound(t *testing.T) {
	var root *Node

	put(key1, newBillContent(1), nil, &root)
	put(key2, newBillContent(2), nil, &root)
	put(key3, newBillContent(3), nil, &root)

	node, found := getNode(root, key4)
	assert.False(t, found)
	assert.Nil(t, node)
}
