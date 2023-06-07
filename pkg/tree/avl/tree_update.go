package avl

import (
	"fmt"
)

// Update updates the given value with given key. If a value with given key
// does not exist, returns an error.
//
// This method should NOT be called concurrently!
func (t *Tree[K, V]) Update(key K, value V) error {
	r, err := update(t.root, key, value)
	if err != nil {
		return err
	}
	t.root = r
	return nil
}

func update[K Key[K], V Value[V]](node *Node[K, V], key K, value V) (*Node[K, V], error) {
	if node == nil {
		return nil, fmt.Errorf("%w: key %v does not exist", ErrNotFound, key)
	}
	if node.clean {
		node = newDirtyNode(node)
	}
	i := node.key.Compare(key)
	if i > 0 {
		left, err := update(node.left, key, value)
		if err != nil {
			return nil, err
		}
		node.left = left
		return node, nil
	} else if i < 0 {
		right, err := update(node.right, key, value)
		if err != nil {
			return nil, err
		}
		node.right = right
		return node, nil
	}
	node.value = value
	return node, nil
}
