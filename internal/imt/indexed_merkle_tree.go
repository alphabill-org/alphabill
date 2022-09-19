package imt

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/holiman/uint256"
)

var ErrNilData = errors.New("merkle tree input data is nil")

type (
	IndexedMerkleTree struct {
		root       *node
		dataLength int // number of leaves
	}

	node struct {
		left  *node
		right *node
		data  *Data
	}

	Data struct {
		Val  []byte
		Hash []byte
		//LeafHash []byte
	}
)

// New creates a new indexed Merkle Tree for given set of leaves.
func New(leaves []*Data, hashAlgorithm crypto.Hash) (*IndexedMerkleTree, error) {
	// TODO data has to be sorted
	if leaves == nil {
		return nil, ErrNilData
	}
	zeroHash := make([]byte, hashAlgorithm.Size())
	return &IndexedMerkleTree{root: createTreeNode(leaves, hashAlgorithm, zeroHash), dataLength: len(leaves)}, nil
}

// GetRootHash returns the root Hash of the Indexed Merkle Tree.
func (s *IndexedMerkleTree) GetRootHash() []byte {
	if s.root == nil {
		return nil
	}
	return s.root.data.Hash
}

// GetMerklePath extracts the merkle path from the given leaf to root.
func (s *IndexedMerkleTree) GetMerklePath(val []byte) ([]*Data, error) {
	var z []*Data
	s.getMerklePath(val, s.root, &z)
	return z, nil
}

// getMerklePath recurisvely extracts the merkle path from given leaf to current node curr.
func (s *IndexedMerkleTree) getMerklePath(val []byte, curr *node, z *[]*Data) {
	if curr.isLeaf() {
		//*z = append([]*Data{{Val: curr.data.Val, Hash: curr.data.LeafHash}}, *z...)
		return
	}
	if bytes.Compare(val, curr.data.Val) <= 0 {
		// target in the left sub-tree, prepend right node, and go left
		*z = append([]*Data{curr.right.data}, *z...)
		s.getMerklePath(val, curr.left, z)
	} else {
		// target in the right sub-tree, prepend left node and go right
		*z = append([]*Data{curr.left.data}, *z...)
		s.getMerklePath(val, curr.right, z)
	}
}

func createTreeNode(data []*Data, hashAlgorithm crypto.Hash, zeroHash []byte) *node {
	if len(data) == 0 {
		return &node{data: &Data{Hash: zeroHash}}
	}
	if len(data) == 1 {
		d := data[0]
		hasher := hashAlgorithm.New()
		hasher.Write([]byte{1}) // leaf hash starts with byte 1 to prevent false proofs
		hasher.Write(d.Val)
		hasher.Write(d.Hash) // d.LeafHAsh

		//fmt.Println(fmt.Sprintf("creating leaf node, hashing: val=%X hash=%X", d.Val, d.LeafHash))
		return &node{data: &Data{
			Val:  d.Val,
			Hash: hasher.Sum(nil),
			//LeafHash: d.LeafHash,
		}}
	}
	n := len(data) / 2
	left := createTreeNode(data[:n], hashAlgorithm, zeroHash)
	right := createTreeNode(data[n:], hashAlgorithm, zeroHash)

	hasher := hashAlgorithm.New()
	hasher.Write([]byte{0}) // non-leaf hash starts with byte 0 to prent false proofs
	hasher.Write(left.data.Hash)
	hasher.Write(right.data.Hash)
	return &node{left: left, right: right, data: &Data{Val: data[n-1].Val, Hash: hasher.Sum(nil)}}
}

// PrettyPrint returns human readable string representation of the Merkle Tree.
func (s *IndexedMerkleTree) PrettyPrint() string {
	if s.root == nil {
		return "tree is empty"
	}
	out := ""
	s.output(s.root, "", false, &out)
	return out
}

func (s *IndexedMerkleTree) output(node *node, prefix string, isTail bool, str *string) {
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
	*str += fmt.Sprintf("%d=%X\n", uint256.NewInt(0).SetBytes(node.data.Val).Uint64(), node.data.Hash)
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

func (n *node) isLeaf() bool {
	return n.left == nil && n.right == nil
}

// EvalMerklePath returns root hash calculated from the given hash chain
func EvalMerklePath(merklePath []*Data, leaf *Data, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()

	hasher.Write([]byte{1})
	hasher.Write(leaf.Val)
	hasher.Write(leaf.Hash)
	h := hasher.Sum(nil)
	hasher.Reset()

	for _, item := range merklePath {
		hasher.Write([]byte{0})
		if bytes.Compare(leaf.Val, item.Val) <= 0 {
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
