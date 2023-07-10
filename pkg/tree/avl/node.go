package avl

import (
	"fmt"
)

type (

	// Node represents a single value in the AVL Tree, with a key of type K and a value of type V.
	// If the node is a leaf, its left and right fields are nil. If the node is an inner node, it
	// has at least one non-nil left or right field.
	//
	// The depth of a leaf node is 1. The depth of an inner node is equal to the maximum depth of
	// its children plus 1. For example, if the left child has a depth of 3 and the right child has
	// a depth of 2, the depth of the inner node would be max(3, 2) + 1 = 3 + 1 = 4. Depth value is
	// used to rebalance the AVL Tree.
	//
	// To enable destructive updates, a node in an AVL tree has a "clean" field. Whenever a new node
	// is added or an existing node is changed (including rotations), a copy of the node is made with
	// the clean field set to false (see Tree.Clone function for more information).
	Node[K Key[K], V Value[V]] struct {
		key   K
		value V
		left  *Node[K, V]
		right *Node[K, V]
		clean bool
		depth int64
	}

	// Key represents the type of the key and is used to insert, update, search, and delete values
	// from the AVL Tree.
	Key[K any] interface {

		// Compare returns a negative integer, zero, or a positive integer as this Key is less than,
		// equal to, or greater than the specified key k.
		Compare(k K) int
	}

	// Value represents a value of the node.
	Value[V any] interface {
		Clone() V
	}

	IntKey int
)

// newLeaf returns a new leaf node with given key and value. The new leaf is marked as dirty to enable destructive
// updates.
func newLeaf[K Key[K], V Value[V]](key K, value V) *Node[K, V] {
	return &Node[K, V]{
		key:   key,
		value: value,
		//	total: value,
		depth: 1, // leaf depth is always 1
		clean: false,
	}
}

// newDirtyNode returns a copy of the node. The new node is marked as dirty (clean = false)
// to enable destructive updates.
func newDirtyNode[K Key[K], V Value[V]](n *Node[K, V]) *Node[K, V] {
	return &Node[K, V]{
		key:   n.key,
		value: n.value,
		depth: n.depth,
		left:  n.left,
		right: n.right,
		clean: false, // we consider a copy dirty to enable destructive updates
	}
}

func (n *Node[K, V]) Depth() int64 {
	if n == nil {
		return 0 // zero == no node
	}
	return n.depth
}

func (n *Node[K, V]) Value() V {
	return n.value
}

func (n *Node[K, V]) Key() K {
	return n.key
}

func (n *Node[K, V]) Clean() bool {
	return n.clean
}

func (n *Node[K, V]) Left() *Node[K, V] {
	if n == nil {
		return nil
	}
	return n.left
}

func (n *Node[K, V]) Right() *Node[K, V] {
	if n == nil {
		return nil
	}
	return n.right
}

func (n *Node[K, V]) String() string {
	return fmt.Sprintf("key=%v, depth=%d, %v, clean=%v", n.key, n.depth, n.value, n.clean)
}

// Compare returns 0 if a == b, 1 if a > b, and -1 if a < b.
func (a IntKey) Compare(b IntKey) int {
	if int(a) == int(b) {
		return 0
	}
	if int(a) > int(b) {
		return 1 // go left
	}
	return -1 // go right
}

func calculateDepth[K Key[K], V Value[V]](n *Node[K, V]) int64 {
	return max(n.left.Depth(), n.right.Depth()) + 1
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
