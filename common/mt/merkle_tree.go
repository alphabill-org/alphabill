package mt

import (
	"crypto"
	"errors"
	"fmt"
)

var ErrIndexOutOfBounds = errors.New("merkle tree data index out of bounds")

type (
	MerkleTree struct {
		root       *node
		dataLength int // number of leaves
	}

	Data interface {
		Hash(hashAlgorithm crypto.Hash) []byte
	}

	// PathItem helper struct for proof extraction, contains Hash and Direction from parent node
	PathItem struct {
		Hash          []byte
		DirectionLeft bool // true - left from parent, false - right from parent
	}

	node struct {
		left  *node
		right *node
		hash  []byte
	}

	// ByteHasher helper struct to satisfy Data interface
	ByteHasher struct {
		Val []byte
	}
)

// New creates a new canonical Merkle Tree.
func New[T Data](hashAlgorithm crypto.Hash, data []T) *MerkleTree {
	if len(data) == 0 {
		return &MerkleTree{root: nil, dataLength: 0}
	}
	return &MerkleTree{root: createMerkleTree(data, hashAlgorithm), dataLength: len(data)}
}

// EvalMerklePath returns root hash calculated from the given leaf and path items
func EvalMerklePath(merklePath []*PathItem, leaf Data, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	h := leaf.Hash(hashAlgorithm)
	for _, item := range merklePath {
		if item.DirectionLeft {
			hasher.Write(h)
			hasher.Write(item.Hash)
		} else {
			hasher.Write(item.Hash)
			hasher.Write(h)
		}
		h = hasher.Sum(nil)
		hasher.Reset()
	}
	return h
}

// PlainTreeOutput calculates the output hash of the chain.
func PlainTreeOutput(merklePath []*PathItem, input []byte, hashAlgorithm crypto.Hash) []byte {
	if len(merklePath) == 0 {
		return input
	}
	hasher := hashAlgorithm.New()
	h := input
	for _, item := range merklePath {
		if item.DirectionLeft {
			hasher.Write(h)
			hasher.Write(item.Hash)
		} else {
			hasher.Write(item.Hash)
			hasher.Write(h)
		}
		h = hasher.Sum(nil)
		hasher.Reset()
	}
	return h
}

// GetRootHash returns the root Hash of the Merkle Tree.
func (s *MerkleTree) GetRootHash() []byte {
	if s.root == nil {
		return nil
	}
	return s.root.hash
}

// GetMerklePath extracts the merkle path from the given leaf to root.
func (s *MerkleTree) GetMerklePath(leafIdx int) ([]*PathItem, error) {
	if leafIdx < 0 || leafIdx >= s.dataLength {
		return nil, ErrIndexOutOfBounds
	}

	var z []*PathItem
	curr := s.root
	b := 0
	m := s.dataLength

	// iteratively descending the tree
	for m > 1 {
		n := hibit(m - 1)
		if leafIdx < b+n { // target in the left sub-tree
			z = append([]*PathItem{{Hash: curr.right.hash, DirectionLeft: true}}, z...)
			curr = curr.left
			m = n
		} else { // target in the right sub-tree
			z = append([]*PathItem{{Hash: curr.left.hash, DirectionLeft: false}}, z...)
			curr = curr.right
			b = b + n
			m = m - n
		}
	}
	return z, nil
}

// PrettyPrint returns human readable string representation of the Merkle Tree.
func (s *MerkleTree) PrettyPrint() string {
	if s.root == nil {
		return "tree is empty"
	}
	out := ""
	s.output(s.root, "", false, &out)
	return out
}

func (s *MerkleTree) output(node *node, prefix string, isTail bool, str *string) {
	if node.right != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "│   "
		} else {
			newPrefix += "    "
		}
		s.output(node.right, newPrefix, false, str)
	}
	*str += prefix
	if isTail {
		*str += "└── "
	} else {
		*str += "┌── "
	}
	*str += fmt.Sprintf("%X\n", node.hash)
	if node.left != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "    "
		} else {
			newPrefix += "│   "
		}
		s.output(node.left, newPrefix, true, str)
	}
}

func createMerkleTree[T Data](data []T, hashAlgorithm crypto.Hash) *node {
	if len(data) == 0 {
		return &node{hash: make([]byte, hashAlgorithm.Size())}
	}
	if len(data) == 1 {
		return &node{hash: data[0].Hash(hashAlgorithm)}
	}
	n := hibit(len(data) - 1)
	hasher := hashAlgorithm.New()
	left := createMerkleTree(data[:n], hashAlgorithm)
	right := createMerkleTree(data[n:], hashAlgorithm)
	hasher.Write(left.hash)
	hasher.Write(right.hash)
	return &node{left: left, right: right, hash: hasher.Sum(nil)}
}

// hibit floating-point-free equivalent of 2**math.floor(math.log(m, 2)),
// could be preferred for larger values of m to avoid rounding errors
func hibit(n int) int {
	if n < 0 {
		panic("hibit function input cannot be negative (merkle tree input data length cannot be zero)")
	}
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n - (n >> 1)
}

func (h *ByteHasher) Hash(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(h.Val)
	return hasher.Sum(nil)
}
