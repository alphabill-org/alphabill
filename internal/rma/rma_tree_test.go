package rma

import (
	"crypto"
	"fmt"
	"hash"
	"math/rand"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	TestData uint64
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

func TestEmpty(t *testing.T) {
	tr, err := New(defaultConfig())
	require.NoError(t, err)
	require.NotNil(t, tr)
	require.Nil(t, tr.GetRootHash())
	require.Nil(t, tr.TotalValue())

	unit, err := tr.get(uint256.NewInt(0))
	require.Error(t, err)
	require.Nil(t, unit)

	exists := tr.exists(uint256.NewInt(9))
	require.False(t, exists)

	require.Error(t, tr.setData(uint256.NewInt(0), TestData(2), nil))
	require.Error(t, tr.setOwner(uint256.NewInt(0), Predicate{1, 2, 3}, nil))
}

func TestHashAlgorithms(t *testing.T) {
	config := defaultConfig()
	tr, err := New(config)
	require.NoError(t, err)
	require.NotNil(t, tr)

	config.HashAlgorithm = crypto.SHA512
	tr, err = New(config)
	require.NoError(t, err)
	require.NotNil(t, tr)

	config.HashAlgorithm = crypto.MD5
	tr, err = New(config)
	require.ErrorContains(t, err, errStrInvalidHashAlgorithm)
	require.Nil(t, tr)
}

func TestOneItem(t *testing.T) {
	tr, _ := New(defaultConfig())

	tr.set(uint256.NewInt(0), Predicate{1, 2, 3}, TestData(100), nil)

	require.NotNil(t, tr.GetRootHash())

	genSum := tr.TotalValue()
	sum, ok := genSum.(Uint64SummaryValue)
	require.True(t, ok, "should be type of TestSummaryValue")
	require.Equal(t, Uint64SummaryValue(100), sum)
}

func TestTwoItems(t *testing.T) {
	tr, _ := New(defaultConfig())

	id := uint256.NewInt(0)
	owner := Predicate{1, 2, 3}
	data := TestData(100)
	stateHash := []byte("state hash")

	tr.set(id, owner, data, stateHash)
	rootHash1 := tr.GetRootHash()
	tr.set(uint256.NewInt(1), Predicate{1, 2, 3, 4}, TestData(100), []byte("state hash 2"))

	rootHash2 := tr.GetRootHash()
	require.NotNil(t, rootHash2)
	require.NotEqual(t, rootHash1, rootHash2)

	genSum := tr.TotalValue()
	sum, ok := genSum.(Uint64SummaryValue)
	require.True(t, ok, "should be type of Uint64SummaryValue")
	require.Equal(t, Uint64SummaryValue(200), sum)

	unit, err := tr.get(id)
	require.NoError(t, err)
	require.Equal(t, owner, unit.Bearer)
	require.Equal(t, data, unit.Data)
	require.Equal(t, stateHash, unit.StateHash)

	exists := tr.exists(id)
	require.True(t, exists)

	exists = tr.exists(uint256.NewInt(9))
	require.False(t, exists)
}

func TestSetOwner(t *testing.T) {
	tr, _ := New(defaultConfig())

	id := uint256.NewInt(0)
	owner1 := Predicate{1, 2, 3}
	owner2 := Predicate{4, 5, 6}
	stateHash1 := []byte("sh 1")
	stateHash2 := []byte("sh 2")

	tr.set(id, owner1, TestData(1), stateHash1)

	err := tr.setOwner(id, owner2, stateHash2)
	require.NoError(t, err)

	actualUnit, err := tr.get(id)
	require.NoError(t, err)

	require.Equal(t, owner2, actualUnit.Bearer)
	require.Equal(t, stateHash2, actualUnit.StateHash)
}

func TestSetData(t *testing.T) {
	tr, _ := New(defaultConfig())

	id := uint256.NewInt(0)
	data1 := TestData(1)
	data2 := TestData(2)
	stateHash1 := []byte("sh 1")
	stateHash2 := []byte("sh 2")

	tr.set(id, Predicate{1, 2, 3}, data1, stateHash1)
	sum1 := tr.TotalValue()
	require.Equal(t, Uint64SummaryValue(1), sum1)

	err := tr.setData(id, data2, stateHash2)
	require.NoError(t, err)

	actualUnit, err := tr.get(id)
	require.NoError(t, err)
	require.Equal(t, data2, actualUnit.Data)
	require.Equal(t, stateHash2, actualUnit.StateHash)

	sum2 := tr.TotalValue()
	require.Equal(t, Uint64SummaryValue(2), sum2)
}

func TestHashing(t *testing.T) {
	config := defaultConfig()
	hashAlgorithm := config.HashAlgorithm
	tr, _ := New(config)

	id := uint256.NewInt(4)
	data := TestData(5)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")
	tr.set(id, owner, data, stateHash)
	actualRootHash := tr.GetRootHash()
	summaryValue := tr.TotalValue()

	var expectedRootHash []byte
	// ID, H(StateHash, H(ID, Bearer, UnitData)), self.SummaryValue, leftChild.hash, leftChild.SummaryValue, rightChild.hash, rightChild.summaryValue)
	h := hashAlgorithm.New()
	idBytes := id.Bytes32()

	// Sub hash H(ID, Bearer, UnitData)
	h.Write(idBytes[:])
	h.Write(owner)
	data.AddToHasher(h)
	hashSub1 := h.Sum(nil)

	// Sub hash H(StateHash, subHash1)
	h.Reset()
	h.Write(stateHash)
	h.Write(hashSub1)
	hashSub2 := h.Sum(nil)

	// Main hash H(ID, subHash2, self.SummaryValue, leftChild.hash, leftChild.SummaryValue, rightChild.hash, rightChild.summaryValue)
	h.Reset()
	h.Write(idBytes[:])
	h.Write(hashSub2)
	summaryValue.AddToHasher(h)
	// Children are empty
	h.Write(make([]byte, hashAlgorithm.Size()))
	h.Write(make([]byte, hashAlgorithm.Size()))
	expectedRootHash = h.Sum(nil)

	require.Equal(t, expectedRootHash, actualRootHash)
}

func TestAddItem(t *testing.T) {
	tr, _ := New(defaultConfig())
	id := uint256.NewInt(4)
	data := TestData(5)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")
	err := tr.AddItem(id, owner, data, stateHash)
	require.NoError(t, err)

	unit, err := tr.get(id)
	requireEqual(t, owner, data, stateHash, unit)

	err = tr.AddItem(id, owner, data, stateHash)
	require.Error(t, err, "Adding existing item must fail")
}

func TestUpdateData(t *testing.T) {
	tr, _ := New(defaultConfig())
	id := uint256.NewInt(4)
	data := TestData(5)
	newData := TestData(6)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")

	updateFunc := func(data UnitData) UnitData {
		return newData
	}

	err := tr.UpdateData(id, updateFunc, stateHash)
	require.Error(t, err, "updating non existing node must fail")

	err = tr.AddItem(id, owner, data, stateHash)
	require.NoError(t, err)

	err = tr.UpdateData(id, updateFunc, stateHash)
	require.NoError(t, err)
	unit, err := tr.get(id)
	require.NoError(t, err)
	requireEqual(t, owner, newData, stateHash, unit)
}

func TestAtomicUpdateOk(t *testing.T) {
	tr, _ := New(defaultConfig())
	id := uint256.NewInt(4)
	data := TestData(5)
	newData := TestData(6)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")

	updateFunc := func(data UnitData) UnitData {
		return newData
	}
	require.Equal(t, 0, len(tr.changes))
	err := tr.AtomicUpdate(AddItem(id, owner, data, stateHash),
		UpdateData(id, updateFunc, stateHash))
	require.NoError(t, err)
	// add and recompute, plus update so 3 changes
	require.True(t, len(tr.changes) >= 2)
	unit, err := tr.get(id)
	require.NoError(t, err)
	// and data unit data is still original data
	requireEqual(t, owner, newData, stateHash, unit)
}

func TestAtomicUpdateRollback(t *testing.T) {
	tr, _ := New(defaultConfig())
	id := uint256.NewInt(4)
	data := TestData(5)
	newData := TestData(6)
	owner := Predicate{1, 2, 3}
	stateHash := []byte("state hash")

	updateFunc := func(data UnitData) UnitData {
		return newData
	}

	err := tr.AtomicUpdate(AddItem(id, owner, data, stateHash))
	require.NoError(t, err)
	require.Equal(t, 1, len(tr.changes))
	err = tr.AtomicUpdate(UpdateData(id, updateFunc, stateHash),
		UpdateData(uint256.NewInt(5), updateFunc, stateHash)) // change non-existing id to generate error
	require.ErrorContains(t, err, "does not exist")
	// both get rolled back, so changes len is still 1
	require.Equal(t, 1, len(tr.changes))
	unit, err := tr.get(id)
	require.NoError(t, err)
	// and data unit data is still original data
	requireEqual(t, owner, data, stateHash, unit)
}

func TestSetNode_Overwrite(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, newNodeContent(1))
	requireNodeEquals(t, at.root, key1, 1)
	at.setNode(key1, newNodeContent(2))
	requireNodeEquals(t, at.root, key1, 2)
}

func TestRevert_Overwrite(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, newNodeContent(1))
	at.Commit()
	treeBefore := at.print()
	at.setNode(key1, newNodeContent(2))
	at.Revert()
	treeAfter := at.print()

	requireTreesEquals(t, treeBefore, treeAfter)
}

func TestAddBill_AVLTreeRotateLeft(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.GetRootHash()
	at.setNode(key3, newNodeContent(3))

	requireNodeEquals(t, at.root, key2, 2)
	requireNodeEquals(t, at.root.Children[0], key1, 1)
	requireNodeEquals(t, at.root.Children[1], key3, 3)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestAddBill_AVLTreeRotateRight(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key3, newNodeContent(3))
	at.setNode(key2, newNodeContent(2))
	at.GetRootHash()
	at.setNode(key1, newNodeContent(1))

	requireNodeEquals(t, at.root, key2, 2)
	requireNodeEquals(t, at.root.Children[0], key1, 1)
	requireNodeEquals(t, at.root.Children[1], key3, 3)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestAddBill_AVLTreeRotateLeftRight(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key10, newNodeContent(10))
	at.setNode(key20, newNodeContent(20))
	at.setNode(key30, newNodeContent(30))
	at.setNode(key1, newNodeContent(1))
	at.setNode(key15, newNodeContent(15))
	at.GetRootHash()
	at.setNode(key12, newNodeContent(12))

	requireNodeEquals(t, at.root, key15, 15)
	requireNodeEquals(t, at.root.Children[0], key10, 10)
	requireNodeEquals(t, at.root.Children[0].Children[0], key1, 1)
	requireNodeEquals(t, at.root.Children[0].Children[1], key12, 12)
	requireNodeEquals(t, at.root.Children[1], key20, 20)
	requireNodeEquals(t, at.root.Children[1].Children[1], key30, 30)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestAddBill_AVLTreeRotateRightLeft(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key10, newNodeContent(10))
	at.setNode(key30, newNodeContent(30))
	at.setNode(key20, newNodeContent(20))

	at.setNode(key25, newNodeContent(25))
	at.setNode(key31, newNodeContent(31))
	at.GetRootHash()
	at.setNode(key24, newNodeContent(24))

	assert.Equal(t, at.root.ID, key25)
	assert.Equal(t, at.root.Children[0].ID, key20)
	assert.Equal(t, at.root.Children[0].Children[0].ID, key10)
	assert.Equal(t, at.root.Children[0].Children[1].ID, key24)
	assert.Equal(t, at.root.Children[1].ID, key30)
	assert.Equal(t, at.root.Children[1].Children[1].ID, key31)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestGetNode_LeftChild(t *testing.T) {
	at, _ := New(defaultConfig())

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
	at, _ := New(defaultConfig())

	at.setNode(key1, nil)
	at.setNode(key2, nil)
	at.setNode(key3, nil)

	node, found := at.getNode(key3)
	assert.True(t, found)
	assert.NotNil(t, node)
}

func TestGetNode_NotFound(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key1, nil)
	at.setNode(key2, nil)
	at.setNode(key3, nil)

	node, found := at.getNode(key4)
	assert.False(t, found)
	assert.Nil(t, node)
}

func TestRemoveNode_empty(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, nil)
	at.removeNode(key1)
	require.Nil(t, at.root)
}

func TestRemoveNode_NonExisting(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, nil)
	at.removeNode(key2)
	require.Equal(t, key1, at.root.ID)
}

func TestRemoveNode_TwoNodes_DeleteLeaf(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.GetRootHash()

	at.removeNode(key2)
	require.True(t, at.exists(key1))
	require.False(t, at.exists(key2))

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestRevert_RemoveNode_TwoNodes_DeleteLeaf(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	hashBefore := at.GetRootHash()
	at.Commit()
	treeBefore := at.print()

	at.removeNode(key2)
	at.Revert()

	requireTreesEquals(t, treeBefore, at.print())

	hashAfter := at.GetRootHash()
	require.Equal(t, hashBefore, hashAfter)
}

func Test_RemoveNode_BiggerTree(t *testing.T) {
	maxNrOfNodes := 25
	createTree := func(nrOfNodes int) *Tree {
		at, _ := New(defaultConfig())
		for i := 0; i < nrOfNodes; i++ {
			at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
		}
		at.Commit()
		at.GetRootHash()
		return at
	}

	// Try all tree sizes up the maxNrOfNodes
	for nrOfNodes := 1; nrOfNodes <= maxNrOfNodes; nrOfNodes++ {
		for i := 0; i < nrOfNodes; i++ {
			idToRemove := uint256.NewInt(uint64(i))
			tree := createTree(nrOfNodes)
			treeBeforeRemoval := tree.print()

			tree.removeNode(idToRemove)
			treeBeforeRootHash := tree.print()

			tree.GetRootHash()
			treeAfterRootHash := tree.print()

			forceRecomputeFullTree(tree)
			treeAfterFullRecalc := tree.print()
			require.Equal(t, treeAfterRootHash, treeAfterFullRecalc,
				"trees are not equal after removing %d and full recalc\n  before removal:\n  %s\n  after removal:\n  %s",
				i,
				treeBeforeRemoval,
				treeBeforeRootHash,
			)
		}
	}
}

func TestRemoveNode_TwoNodes_DeleteRoot(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.GetRootHash()

	at.removeNode(key1)
	require.False(t, at.exists(key1))
	require.True(t, at.exists(key2))

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestRevert_RemoveNode_TwoNodes_DeleteRoot(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.GetRootHash()
	treeBefore := at.print()

	at.Commit()
	at.removeNode(key1)
	at.Revert()

	requireTreesEquals(t, treeBefore, at.print())
	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestRemoveNode_Leaf(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.setNode(key3, newNodeContent(3))
	at.GetRootHash()

	at.removeNode(key3)
	require.True(t, at.exists(key1))
	require.True(t, at.exists(key2))
	require.False(t, at.exists(key3))

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestRevert_RemoveNode_Leaf(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.setNode(key3, newNodeContent(3))
	at.GetRootHash()
	treeBefore := at.print()

	at.Commit()
	at.removeNode(key3)
	at.Revert()

	requireTreesEquals(t, treeBefore, at.print())
	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestRemoveNode_Top(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.setNode(key3, newNodeContent(3))
	at.GetRootHash()

	at.removeNode(key2)
	require.True(t, at.exists(key1))
	require.False(t, at.exists(key2))
	require.True(t, at.exists(key3))

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestRevert_RemoveNode_Top(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.setNode(key3, newNodeContent(3))
	at.GetRootHash()
	treeBefore := at.print()

	at.Commit()
	at.removeNode(key2)
	at.Revert()

	requireTreesEquals(t, treeBefore, at.print())
	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_RightRight(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 11; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make the right-right child node subtree is the highest.
	at.removeNode(key5)
	at.removeNode(key7)

	// Trigger rotation by deleting left sub node children.
	at.removeNode(key3)
	at.GetRootHash()
	at.removeNode(key1)

	// Node 4 became child and 6 was moved under it. A left rotation on node4.
	require.Equal(t, key8, at.root.ID)
	require.Equal(t, key4, at.root.Children[0].ID)
	require.Equal(t, key6, at.root.Children[0].Children[1].ID)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRevert_RemoveNode_RightRight(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 11; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make the right-right child node subtree is the highest.
	at.removeNode(key5)
	at.removeNode(key7)
	at.removeNode(key3)

	at.GetRootHash()
	treeBefore := at.print()
	// Trigger rotation by deleting left sub node children.
	at.Commit()
	at.removeNode(key1)
	at.Revert()

	treeAfter := at.print()
	requireTreesEquals(t, treeBefore, treeAfter)
	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_RightLeft(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 11; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make right-left child node subtree the highest.
	at.removeNode(key10)
	at.removeNode(key1)
	at.GetRootHash()

	// Trigger rotation by deleting left sub node children.
	// Rotate right 8
	// Rotate left 4
	//println(at.print())
	at.removeNode(key3)
	//println(at.print())

	// Node 6 becomes the root by two rotations.
	require.Equal(t, key6, at.root.ID)
	require.Equal(t, key8, at.root.Children[1].ID)
	require.Equal(t, key4, at.root.Children[0].ID)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRevert_RemoveNode_RightLeft(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 11; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make right-left child node subtree the highest.
	at.removeNode(key10)
	at.removeNode(key1)
	at.GetRootHash()
	treeBefore := at.print()

	// Trigger rotation by deleting left sub node children.
	at.Commit()
	at.removeNode(key3)
	at.Revert()

	treeAfter := at.print()
	requireTreesEquals(t, treeBefore, treeAfter)
	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_LeftLeft(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 14; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make left-left child node subtree the highest.
	at.removeNode(key11)
	at.removeNode(key13)
	at.removeNode(key7)
	at.removeNode(key5)

	// Trigger balancing by deleting sub nodes from right child.
	at.removeNode(key12)
	at.GetRootHash()
	at.removeNode(key9)

	// Node 4 becomes the root by one rotation.
	require.Equal(t, key4, at.root.ID)
	require.Equal(t, key8, at.root.Children[1].ID)
	require.Equal(t, key6, at.root.Children[1].Children[0].ID)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRevert_RemoveNode_LeftLeft(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 14; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make left-left child node subtree the highest.
	at.removeNode(key11)
	at.removeNode(key13)
	at.removeNode(key7)
	at.removeNode(key5)
	at.removeNode(key12)
	at.GetRootHash()

	treeBefore := at.print()

	// Trigger balancing by deleting sub nodes from right child.
	at.Commit()
	at.removeNode(key9)
	at.Revert()

	treeAfter := at.print()
	requireTreesEquals(t, treeBefore, treeAfter)
	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRemoveNode_LeftRight(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 14; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make left-left child node subtree the highest.
	at.removeNode(key11)
	at.removeNode(key13)
	at.removeNode(key3)
	at.removeNode(key1)
	at.removeNode(key12)

	// Trigger balancing by deleting sub nodes from right child.
	at.GetRootHash()
	at.removeNode(key9)

	// Node 6 was becomes the root by two rotations.
	require.Equal(t, key6, at.root.ID)
	require.Equal(t, key8, at.root.Children[1].ID)
	require.Equal(t, key7, at.root.Children[1].Children[0].ID)

	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

// Test cases based on: https://www.geeksforgeeks.org/avl-tree-set-2-deletion/
func TestRevert_RemoveNode_LeftRight(t *testing.T) {
	at, _ := New(defaultConfig())

	for i := 1; i < 14; i++ {
		at.setNode(uint256.NewInt(uint64(i)), newNodeContent(i))
	}

	// Make left-left child node subtree the highest.
	at.removeNode(key11)
	at.removeNode(key13)
	at.removeNode(key3)
	at.removeNode(key1)
	at.removeNode(key12)
	at.GetRootHash()
	treeBefore := at.print()

	// Trigger balancing by deleting sub nodes from right child.
	at.Commit()
	at.removeNode(key9)
	at.Revert()

	treeAfter := at.print()
	requireTreesEquals(t, treeBefore, treeAfter)
	actualRootHash := at.GetRootHash()
	require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
}

func TestRevert_FirstNode(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key1, newNodeContent(1))
	require.NotNil(t, at.root)
	require.Equal(t, key1, at.root.ID)
	require.NotNil(t, at.GetRootHash())

	at.Revert()
	require.Nil(t, at.root)
	require.Nil(t, at.GetRootHash())
}

func TestRevert_SingleRotation(t *testing.T) {
	at, _ := New(defaultConfig())
	at.setNode(key1, newNodeContent(1))
	at.setNode(key2, newNodeContent(2))
	at.GetRootHash()
	at.Commit()
	treeBefore := at.print()
	require.Equal(t, key1, at.root.ID)
	require.Equal(t, 1, at.root.balance)

	at.setNode(key3, nil)
	require.Equal(t, key2, at.root.ID)

	at.Revert()
	treeAfter := at.print()
	requireTreesEquals(t, treeBefore, treeAfter)
}

func TestRevert_DoubleRotation(t *testing.T) {
	at, _ := New(defaultConfig())

	at.setNode(key10, newNodeContent(10))
	at.setNode(key20, newNodeContent(20))
	at.setNode(key30, newNodeContent(30))
	at.setNode(key1, newNodeContent(1))
	at.setNode(key15, newNodeContent(15))
	at.GetRootHash()
	at.Commit()
	treeBefore := at.print()
	at.setNode(key12, newNodeContent(12))

	at.Revert()
	treeAfter := at.print()
	requireTreesEquals(t, treeBefore, treeAfter)
}

func TestRevert_Fuzzier(t *testing.T) {
	// Run the randomized test 10 times
	for k := 0; k < 10; k++ {
		at, _ := New(defaultConfig())

		var ids []*uint256.Int

		// Create a random initial tree
		nrOfNodes := 25 + rand.Intn(50)
		for i := 0; i < nrOfNodes; i++ {
			id := uint256.NewInt(rand.Uint64())
			ids = append(ids, id)
			at.setNode(id, randomUnit())
		}
		at.GetRootHash()
		at.Commit()
		treeBefore := at.print()

		for i := 0; i < 30; i++ {
			switch rand.Intn(3) {
			case 0:
				fallthrough
			case 1:
				at.setNode(uint256.NewInt(rand.Uint64()), randomUnit())
			case 2:
				idToRemove := rand.Int63n(int64(len(ids)))
				at.removeNode(ids[idToRemove])
			}
		}
		calcRootBeforeRevert := k%2 == 0
		if calcRootBeforeRevert {
			// Commit must not affect revert. The tree computation will be reverted as well.
			at.GetRootHash()
		}

		at.Revert()
		treeAfter := at.print()

		requireTreesEquals(t, treeBefore, treeAfter, fmt.Sprintf("Commit before revert: %v", calcRootBeforeRevert))
		actualRootHash := at.GetRootHash()
		require.Equal(t, forceRecomputeFullTree(at), actualRootHash)
	}
}

func TestGetRootHash_Fuzzier(t *testing.T) {
	// Run the randomized test 10 times
	for k := 0; k < 10; k++ {
		at, _ := New(defaultConfig())

		var ids []*uint256.Int

		// Create a random initial tree
		nrOfNodes := 25 + rand.Intn(50)
		for i := 0; i < nrOfNodes; i++ {
			id := uint256.NewInt(rand.Uint64())
			ids = append(ids, id)
			at.setNode(id, randomUnit())
		}
		at.Commit()
		at.GetRootHash()

		for i := 0; i < 30; i++ {
			treeBeforeTx := at.print()
			var transaction string
			switch rand.Int31n(3) {
			case 0:
				fallthrough
			case 1:
				idToAdd := rand.Uint64()
				transaction = fmt.Sprintf("add item %d", idToAdd)
				at.setNode(uint256.NewInt(idToAdd), randomUnit())
			case 2:
				idToRemove := rand.Int63n(int64(len(ids)))
				transaction = fmt.Sprintf("remove item %d (%d)", ids[idToRemove], idToRemove)
				at.removeNode(ids[idToRemove])
			}
			// The root hash calculation must equal to a force recalculation.
			beforeGetRootHash := at.print()

			at.GetRootHash()
			treeAfterGetRootHash := at.print()

			forceRecomputeFullTree(at)
			treeAfterRecalculation := at.print()

			require.Equal(t, treeAfterGetRootHash, treeAfterRecalculation,
				"trees not equal after full recalculation\n  transaction: %s\n  before transaction:\n  %s\n  before GetRootHash:\n  %s\n  after GetRootHash:\n  %s\n  after full recalculation:\n  %s",
				transaction,
				treeBeforeTx,
				beforeGetRootHash,
				treeAfterGetRootHash,
				treeAfterRecalculation,
			)
			requireTreesEquals(t, treeAfterGetRootHash, treeAfterRecalculation)
		}
	}
}

func (u TestData) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(u)))
}

func (u TestData) Value() SummaryValue {
	return Uint64SummaryValue(u)
}

func requireEqual(t *testing.T, expectedOwner Predicate, expectedData TestData, expectedStateHash []byte, actualUnit *Unit) {
	require.Equal(t, expectedOwner, actualUnit.Bearer)
	require.Equal(t, expectedData, actualUnit.Data)
	require.Equal(t, expectedStateHash, actualUnit.StateHash)
}

func requireNodeEquals(t *testing.T, node *Node, key *uint256.Int, val int) {
	require.Equal(t, key, node.ID)
	value, ok := node.Content.Data.(TestData)
	require.True(t, ok, "should be TestData, as inserted")
	require.Equal(t, TestData(val), value)
	require.Equal(t, Predicate{byte(val)}, node.Content.Bearer)

	require.Equal(t, []byte{byte(val)}, node.Content.StateHash)
}

func requireTreesEquals(t *testing.T, before, after string, msgAndArgs ...string) {
	var m string
	if msgAndArgs != nil {
		m = fmt.Sprintf(msgAndArgs[0], msgAndArgs[1:])
	}
	require.Equal(t, before, after, "trees not equal\n  was: %s\n  now: %s\n  %s", before, after, m)
}

func newNodeContent(val int) *Unit {
	return &Unit{
		Bearer:    Predicate{byte(val)},
		Data:      TestData(uint64(val)),
		StateHash: []byte{byte(val)},
	}
}

// forceRecomputeFullTree recomputes the full tree and returns the root hash
func forceRecomputeFullTree(at *Tree) []byte {
	if at.root != nil {
		setRecomputeTrue(at.root)
	}
	return at.GetRootHash()
}

// setRecomputeTrue Sets recompute true for all the nodes in the tree.
func setRecomputeTrue(node *Node) {
	node.recompute = true
	if node.Children[0] != nil {
		setRecomputeTrue(node.Children[0])
	}
	if node.Children[1] != nil {
		setRecomputeTrue(node.Children[1])
	}
}

func randomUnit() *Unit {
	return &Unit{
		Bearer:    test.RandomBytes(10),
		Data:      TestData(rand.Int31n(1_000_000)),
		StateHash: test.RandomBytes(10),
	}
}
