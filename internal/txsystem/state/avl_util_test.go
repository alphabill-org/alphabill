package state

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/holiman/uint256"
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

func TestSetNode_Overwrite(t *testing.T) {
	at := &avlTree{}
	at.setNode(key1, newNodeContent(1))
	requireNodeEquals(t, at.root, key1, 1)
	at.setNode(key1, newNodeContent(2))
	requireNodeEquals(t, at.root, key1, 2)
}

func TestRevert_Overwrite(t *testing.T) {
	at := &avlTree{recordingEnabled: true}
	at.setNode(key1, newNodeContent(1))
	at.resetChanges()
	treeBefore := at.print()
	at.setNode(key1, newNodeContent(2))
	at.revertChanges()
	treeAfter := at.print()

	requireTreesEquals(t, treeBefore, treeAfter)
}

func TestAddBill_AVLTreeRotateLeft(t *testing.T) {
	at := &avlTree{}

	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.setNode(key3, newNodeContent(3))

	requireNodeEquals(t, at.root, key2, 2)
	requireNodeEquals(t, at.root.Children[0], key1, 1)
	requireNodeEquals(t, at.root.Children[1], key3, 3)
}

func TestAddBill_AVLTreeRotateRight(t *testing.T) {
	at := &avlTree{}

	at.setNode(key3, newNodeContent(3))
	at.setNode(key2, newNodeContent(2))
	at.setNode(key1, newNodeContent(1))

	requireNodeEquals(t, at.root, key2, 2)
	requireNodeEquals(t, at.root.Children[0], key1, 1)
	requireNodeEquals(t, at.root.Children[1], key3, 3)
}

func TestAddBill_AVLTreeRotateLeftRight(t *testing.T) {
	at := &avlTree{}

	at.setNode(key10, newNodeContent(10))
	at.setNode(key20, newNodeContent(20))
	at.setNode(key30, newNodeContent(30))
	at.setNode(key1, newNodeContent(1))
	at.setNode(key15, newNodeContent(15))
	at.setNode(key12, newNodeContent(12))

	requireNodeEquals(t, at.root, key15, 15)
	requireNodeEquals(t, at.root.Children[0], key10, 10)
	requireNodeEquals(t, at.root.Children[0].Children[0], key1, 1)
	requireNodeEquals(t, at.root.Children[0].Children[1], key12, 12)
	requireNodeEquals(t, at.root.Children[1], key20, 20)
	requireNodeEquals(t, at.root.Children[1].Children[1], key30, 30)
}

func TestAddBill_AVLTreeRotateRightLeft(t *testing.T) {
	at := &avlTree{}

	at.setNode(key10, nil)
	at.setNode(key30, nil)
	at.setNode(key20, nil)

	at.setNode(key25, nil)
	at.setNode(key31, nil)
	at.setNode(key24, nil)

	assert.Equal(t, at.root.ID, key25)
	assert.Equal(t, at.root.Children[0].ID, key20)
	assert.Equal(t, at.root.Children[0].Children[0].ID, key10)
	assert.Equal(t, at.root.Children[0].Children[1].ID, key24)
	assert.Equal(t, at.root.Children[1].ID, key30)
	assert.Equal(t, at.root.Children[1].Children[1].ID, key31)
}

func TestGetNode_LeftChild(t *testing.T) {
	at := &avlTree{}

	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.setNode(key3, newNodeContent(3))

	node, found := at.getNode(key1)
	assert.True(t, found)
	assert.NotNil(t, node)
	requireNodeEquals(t, node, key1, 1)
	assert.Nil(t, node.Children[0])
	assert.Nil(t, node.Children[1])
}

func TestGetNode_RightChild(t *testing.T) {
	at := &avlTree{}

	at.setNode(key1, nil)
	at.setNode(key2, nil)
	at.setNode(key3, nil)

	node, found := at.getNode(key3)
	assert.True(t, found)
	assert.NotNil(t, node)
}

func TestGetNode_NotFound(t *testing.T) {
	at := &avlTree{}

	at.setNode(key1, nil)
	at.setNode(key2, nil)
	at.setNode(key3, nil)

	node, found := at.getNode(key4)
	assert.False(t, found)
	assert.Nil(t, node)
}

func TestRevert_FirstNode(t *testing.T) {
	at := &avlTree{recordingEnabled: true}

	at.setNode(key1, newNodeContent(1))
	require.NotNil(t, at.root)
	require.Equal(t, key1, at.root.ID)

	at.revertChanges()
	require.Nil(t, at.root)
}

func TestRevert_SingleRotation(t *testing.T) {
	at := &avlTree{recordingEnabled: true}
	at.setNode(key1, nil)
	at.setNode(key2, nil)
	treeBefore := at.print()
	at.resetChanges()
	require.Equal(t, key1, at.root.ID)
	require.Equal(t, 1, at.root.balance)

	at.setNode(key3, nil)
	require.Equal(t, key2, at.root.ID)

	at.revertChanges()
	treeAfter := at.print()
	requireTreesEquals(t, treeBefore, treeAfter)
}

func TestRevert_DoubleRotation(t *testing.T) {
	at := &avlTree{recordingEnabled: true}

	at.setNode(key10, newNodeContent(10))
	at.setNode(key20, newNodeContent(20))
	at.setNode(key30, newNodeContent(30))
	at.setNode(key1, newNodeContent(1))
	at.setNode(key15, newNodeContent(15))
	treeBefore := at.print()
	at.resetChanges()
	at.setNode(key12, newNodeContent(12))
	at.revertChanges()
	treeAfter := at.print()

	requireTreesEquals(t, treeBefore, treeAfter)
}

func requireNodeEquals(t *testing.T, node *Node, key *uint256.Int, val int) {
	require.Equal(t, key, node.ID)
	value, ok := node.Content.Data.(TestData)
	require.True(t, ok, "should be TestData, as inserted")
	require.Equal(t, TestData(val), value)
	require.Equal(t, Predicate{byte(val)}, node.Content.Bearer)

	require.Equal(t, []byte{byte(val)}, node.Content.StateHash)
}

func requireTreesEquals(t *testing.T, before, after string) {
	require.Equal(t, before, after, "trees not equals after revert\n was: %s\n now: %s", before, after)
}

func newNodeContent(val int) *Unit {
	return &Unit{
		Bearer:    Predicate{byte(val)},
		Data:      TestData(uint64(val)),
		StateHash: []byte{byte(val)},
	}
}

func printTree(at *avlTree) {
	out := ""
	at.output(at.root, "", false, &out)
	println(out)
}
