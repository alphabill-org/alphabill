package avl

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("node already exists")
)

// Get looks for the value with given key in the AVL tree, returning it.
// Returns nil if unable to find that value.
func (t *Tree[K, V]) Get(key K) (v V, err error) {
	if t.root == nil {
		return v, fmt.Errorf("item %v does not exist: %w", key, ErrNotFound)
	}
	node := get[K, V](t.root, key)
	if node == nil {
		return v, fmt.Errorf("item %v does not exist: %w", key, ErrNotFound)
	}
	return node.value, nil
}

func get[K Key[K], V Value[V]](node *Node[K, V], key K) *Node[K, V] {
	if node == nil {
		return nil
	}
	i := node.key.Compare(key)
	if i > 0 {
		return get(node.left, key)
	} else if i < 0 {
		return get(node.right, key)
	}
	return node
}
