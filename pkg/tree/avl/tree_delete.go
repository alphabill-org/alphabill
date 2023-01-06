package avl

import "fmt"

// Delete removes a node with the given key from the AVL Tree.
// If no such value exists, returns an error.
//
// This method should NOT be called concurrently!
func (t *Tree[K, V]) Delete(key K) error {
	node, err := delete[K, V](t.root, key)
	if err != nil {
		return err
	}
	t.root = node
	return nil
}

func delete[K Key[K], V Value[V]](node *Node[K, V], key K) (*Node[K, V], error) {
	if node == nil {
		return nil, fmt.Errorf("key %v does not exist", key)
	}
	if node.clean {
		node = newDirtyNode[K, V](node)
	}

	i := node.key.Compare(key)
	if i > 0 {
		// go to left subtree
		left, err := delete(node.left, key)
		if err != nil {
			return nil, err
		}
		node.left = left
	} else if i < 0 {
		// go to right subtree
		right, err := delete(node.right, key)
		if err != nil {
			return nil, err
		}
		node.right = right
	} else {
		// keys are equal
		if node.left == nil && node.right == nil {
			// node is a leaf
			return nil, nil
		}

		if node.left == nil {
			// node has only right subtree.
			return node.right, nil
		} else if node.right == nil {
			// node has only left subtree
			return node.left, nil
		}
		// Replace the node with its in-order predecessor (e.g. the largest key that is smaller than node.key).
		// The first move is always to the left followed by moves to the right until a node without a right child is
		// found.
		dirtyLeftNode := newDirtyNode(node.left)
		newLeft, predecessor := replace(dirtyLeftNode.right, dirtyLeftNode)
		node.key = predecessor.key
		node.value = predecessor.value
		// because of rotations we need to update left child.
		node.left = newLeft
		node.depth = calculateDepth(node)
		return rotate(node), nil

	}
	node.depth = calculateDepth(node)
	node = rotate(node)
	return node, nil
}

func replace[K Key[K], V Value[V]](node *Node[K, V], parent *Node[K, V]) (*Node[K, V], *Node[K, V]) {
	if node == nil {
		// parent is the predecessor
		return parent.left, parent
	}
	if node.clean {
		node = newDirtyNode(node)
		parent.right = node
	}
	var predecessor *Node[K, V]
	var replacedNode *Node[K, V]
	if node.right != nil {
		// always go to right subtree until a predecessor is found
		replacedNode, predecessor = replace(node.right, node)
	} else {
		predecessor = node
		parent.right = predecessor.left
		parent.depth = calculateDepth(parent)
		// the depth changes at only nodes between the root and the predecessor parent node.
		return rotate(parent), predecessor
	}
	parent.right = replacedNode
	parent.depth = calculateDepth(parent)
	// the depth changes at only nodes between the root and the predecessor parent node.
	parent = rotate(parent)
	return parent, predecessor
}
