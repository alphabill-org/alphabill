package mt

import (
	"crypto"
	"fmt"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var ErrNilData = errors.New("merkle tree input data is nil")

type (
	MerkleTree struct {
		root *node
	}

	Data interface {
		Hash(hashAlgorithm crypto.Hash) []byte
	}

	node struct {
		left  *node
		right *node
		hash  []byte
	}
)

// New creates a new canonical Merkle Tree.
func New(hashAlgorithm crypto.Hash, data []Data) (*MerkleTree, error) {
	if data == nil {
		return nil, ErrNilData
	}
	return &MerkleTree{root: createMerkleTree(data, hashAlgorithm)}, nil
}

// GetRootHash returns the root hash of the Merkle Tree.
func (s *MerkleTree) GetRootHash() []byte {
	if s.root == nil {
		return nil
	}
	return s.root.hash
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

func createMerkleTree(data []Data, hashAlgorithm crypto.Hash) *node {
	if len(data) == 0 {
		return nil
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
