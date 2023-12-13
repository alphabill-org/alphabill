package avl

import (
	"fmt"
)

// String returns a string of the AVL Tree.
// Should not be used to print out large trees.
func (t *Tree[K, V]) String() string {
	if t == nil || t.root == nil {
		return "────┤ empty"
	}
	return print(t.root, "", false, true)
}

func print[K Key[K], V Value[V]](node *Node[K, V], prefix string, tail bool, isRoot bool) (str string) {
	if node.right != nil {
		str += print(node.right, rightNodePrefix(prefix, tail), false, false)
	}
	str += fmt.Sprintf("%s─┤ %v\n", perf(prefix, isRoot, tail), node)
	if node.left != nil {
		str += print(node.left, leftNodePrefix(prefix, tail, isRoot), true, false)
	}
	return
}

func perf(prefix string, isRoot bool, tail bool) string {
	if isRoot {
		return prefix + "───"
	} else if tail {
		return prefix + "└──"
	}
	return prefix + "┌──"
}

func rightNodePrefix(prefix string, tail bool) string {
	if tail {
		return prefix + "│\t"
	}
	return prefix + "\t"
}

func leftNodePrefix(prefix string, tail bool, isRoot bool) string {
	if tail || isRoot {
		return prefix + "\t"
	}
	return prefix + "│\t"
}
