package avl

import (
	"fmt"
)

// Add inserts the given value to the tree. The value is first inserted into
// the tree in the appropriate position as determined by the rules of a binary
// search tree. If the insertion causes the tree to become unbalanced, one or
// more tree rotations are performed to restore balance to the tree.
//
// If a value with given key exists, returns an error.
//
// This method should NOT be called concurrently!
func (t *Tree[K, V]) Add(key K, value V) error {
	node, err := insert(t.root, key, value)
	if err != nil {
		return err
	}
	t.root = node
	return nil
}

func insert[K Key[K], V Value[V]](p *Node[K, V], key K, value V) (*Node[K, V], error) {
	if p == nil {
		return newLeaf[K, V](key, value), nil
	}
	i := p.key.Compare(key)
	if i == 0 {
		return nil, fmt.Errorf("%w: key %v exists", ErrAlreadyExists, key)
	}
	if p.clean {
		// if node is clean then make a dirty clone of it
		p = newDirtyNode(p)
	}
	if i > 0 {
		// left child
		l, err := insert(p.left, key, value)
		if err != nil {
			return nil, err
		}
		p.left = l
	} else {
		// right child
		right, err := insert(p.right, key, value)
		if err != nil {
			return nil, err
		}
		p.right = right
	}
	return rotate(p), nil
}

func rotate[K Key[K], V Value[V]](p *Node[K, V]) *Node[K, V] {
	ld, rd := p.left.Depth(), p.right.Depth()
	if ld > rd+1 {
		lld, lrd := p.left.left.Depth(), p.left.right.Depth()
		if lld < lrd {
			p.left = rotateLeft(p.left)
		}
		p = rotateRight(p)
	}
	if rd > ld+1 {
		rld, rrd := p.right.left.Depth(), p.right.right.Depth()
		if rld > rrd {
			p.right = rotateRight(p.right)
		}
		p = rotateLeft(p)
	}
	p.depth = calculateDepth(p)
	return p
}

func rotateRight[K Key[K], V Value[V]](node *Node[K, V]) *Node[K, V] {
	tmp := node.left
	if node.clean {
		node = newDirtyNode(node)
	}
	if tmp.clean {
		tmp = newDirtyNode(tmp)
	}
	node.left = tmp.right
	node.depth = calculateDepth(node)
	tmp.right = node
	return tmp
}

func rotateLeft[K Key[K], V Value[V]](node *Node[K, V]) *Node[K, V] {
	tmp := node.right
	if node.clean {
		node = newDirtyNode(node)
	}
	if tmp.clean {
		tmp = newDirtyNode(tmp)
	}
	node.right = tmp.left
	node.depth = calculateDepth(node)
	tmp.left = node
	return tmp
}
