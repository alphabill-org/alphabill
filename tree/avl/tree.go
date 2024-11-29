package avl

type (
	// Tree represents an AVL tree.
	//
	// This implementation is not safe for concurrent use by multiple goroutines. If multiple
	// goroutines access a tree concurrently, and at least one of them modifies the tree, it
	// must be synchronized externally.
	//
	// Clone method may be used to make a copy of the tree (lazily). The original tree and the
	// cloned tree can be used by different goroutines.
	//
	// Tree is indexed according to the function Key.Compare result. Node value V can be any
	// struct that implements Value interface.
	Tree[K Key[K], V Value[V]] struct {
		root      *Node[K, V]
		traverser Traverser[K, V]
	}

	// Traverser is an interface to traverse Tree.
	Traverser[K Key[K], V Value[V]] interface {
		Traverse(n *Node[K, V]) error
	}

	// PostOrderCommitTraverser is a Traverser that visits all non-clean nodes and
	// sets the clean value to true.
	PostOrderCommitTraverser[K Key[K], V Value[V]] struct{}
)

// New returns an empty AVL tree.
// - K is the type of the node's key (e.g. IntKey)
// - V is the type of the node's data (e.g. any kind of value that implements Value interface)
func New[K Key[K], V Value[V]]() *Tree[K, V] {
	return NewWithTraverser[K, V](&PostOrderCommitTraverser[K, V]{})
}

// NewWithTraverser creates a new AVL tree with a custom Traverser.
func NewWithTraverser[K Key[K], V Value[V]](traverser Traverser[K, V]) *Tree[K, V] {
	return &Tree[K, V]{traverser: traverser}
}

// NewWithTraverserAndRoot creates a new AVL tree with a custom Traverser and a balanced root node.
func NewWithTraverserAndRoot[K Key[K], V Value[V]](traverser Traverser[K, V], root *Node[K, V]) *Tree[K, V] {
	return &Tree[K, V]{root: root, traverser: traverser}
}

// Clone clones the AVL tree, lazily.
//
// This method should NOT be called concurrently!
//
// The original tree and the cloned tree can be used by different goroutines.
// Writes to both old tree and cloned tree use copy-on-write logic, creating
// new nodes whenever one of nodes would have been modified. Read operations
// should have no performance degradation. Write operations will initially
// experience minor slow-downs caused by additional allocs and copies due to
// the aforementioned copy-on-write logic, but should converge to the original
// performance characteristics of the original tree.
func (t *Tree[K, V]) Clone() *Tree[K, V] {
	return &Tree[K, V]{root: t.root, traverser: t.traverser}
}

// IsClean returns true if t does not contain uncommitted changes, false otherwise.
func (t *Tree[K, V]) IsClean() bool {
	if t.root == nil {
		return true
	}
	return t.root.clean
}

// Commit saves the changes made to the tree.
func (t *Tree[K, V]) Commit() error {
	return t.traverser.Traverse(t.root)
}

func (t *Tree[K, V]) Root() *Node[K, V] {
	return t.root
}

// Traverse traverses the given tree with the given traverser. Does nothing if the given traverser is nil.
func (t *Tree[K, V]) Traverse(traverser Traverser[K, V]) error {
	if traverser == nil {
		return nil
	}
	return traverser.Traverse(t.root)
}

func (p *PostOrderCommitTraverser[K, V]) Traverse(n *Node[K, V]) error {
	if n == nil || n.clean {
		return nil
	}
	if err := p.Traverse(n.left); err != nil {
		return err
	}
	if err := p.Traverse(n.right); err != nil {
		return err
	}
	p.SetClean(n)
	return nil
}

func (p *PostOrderCommitTraverser[K, V]) SetClean(n *Node[K, V]) {
	if n == nil || n.clean {
		return
	}
	n.clean = true
}
