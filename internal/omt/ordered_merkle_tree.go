package omt

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"hash"
)

var ErrNilData = errors.New("merkle tree input data is nil")

type (
	OrderedMerkleTree struct {
		root       *node
		dataLength int // number of leaves
	}

	node struct {
		left  *node
		right *node
		data  *Data
	}

	Data struct {
		Val      []byte
		Hash     []byte // Hash accumulated tree hash, leaf nodes have prefix 1, non-leaf nodes have prefix 0
		leafHash []byte // leafHash for leaf nodes, proof-chain first element needs to be raw hash not tree hash
	}
)

// New creates a new ordered Merkle Tree for given set of leaves. The leaves have to be sorted.
func New(leaves []*Data, hashAlgorithm crypto.Hash) (*OrderedMerkleTree, error) {
	if leaves == nil {
		return nil, ErrNilData
	}
	zeroHash := make([]byte, hashAlgorithm.Size())
	return &OrderedMerkleTree{root: createTreeNode(leaves, hashAlgorithm, zeroHash), dataLength: len(leaves)}, nil
}

// GetRootHash returns the root Hash of the Ordered Merkle Tree.
func (s *OrderedMerkleTree) GetRootHash() []byte {
	if s.root == nil {
		return nil
	}
	return s.root.data.Hash
}

// GetMerklePath extracts the merkle path from the given leaf to root.
func (s *OrderedMerkleTree) GetMerklePath(val []byte) ([]*Data, error) {
	var z []*Data
	s.getMerklePath(val, s.root, &z)
	return z, nil
}

// getMerklePath recurisvely extracts the merkle path from given leaf to current node curr.
func (s *OrderedMerkleTree) getMerklePath(val []byte, curr *node, z *[]*Data) {
	if curr.isLeaf() {
		*z = append([]*Data{{Val: curr.data.Val, Hash: curr.data.leafHash}}, *z...)
		return
	}
	if bytes.Compare(val, curr.data.Val) <= 0 {
		// target in the left sub-tree, prepend <curr.val,right.hash>, and go left
		*z = append([]*Data{{Val: curr.data.Val, Hash: curr.right.data.Hash}}, *z...)
		s.getMerklePath(val, curr.left, z)
	} else {
		// target in the right sub-tree, prepend <curr.val,left.hash>, and go right
		*z = append([]*Data{{Val: curr.data.Val, Hash: curr.left.data.Hash}}, *z...)
		s.getMerklePath(val, curr.right, z)
	}
}

func createTreeNode(data []*Data, hashAlgorithm crypto.Hash, zeroHash []byte) *node {
	if len(data) == 0 {
		return &node{data: &Data{Hash: zeroHash}}
	}
	hasher := hashAlgorithm.New()
	if len(data) == 1 {
		d := data[0]
		return &node{data: &Data{
			Val:      d.Val,
			Hash:     computeLeafTreeHash(d, hasher),
			leafHash: d.Hash,
		}}
	}
	n := len(data) / 2
	left := createTreeNode(data[:n], hashAlgorithm, zeroHash)
	right := createTreeNode(data[n:], hashAlgorithm, zeroHash)

	hasher.Write([]byte{0})     // non-leaf hash starts with byte 0 to prevent false proofs
	hasher.Write(data[n-1].Val) // include unit value to prevent potential proof manipulation attacks
	hasher.Write(left.data.Hash)
	hasher.Write(right.data.Hash)
	return &node{
		left:  left,
		right: right,
		data:  &Data{Val: data[n-1].Val, Hash: hasher.Sum(nil)}, // val=highest value in left subtree
	}
}

func computeLeafTreeHash(d *Data, hasher hash.Hash) []byte {
	hasher.Write([]byte{1}) // leaf hash starts with byte 1 to prevent false proofs
	hasher.Write(d.Val)
	hasher.Write(d.Hash)
	return hasher.Sum(nil)
}

// PrettyPrint returns human readable string representation of the Merkle Tree.
func (s *OrderedMerkleTree) PrettyPrint() string {
	if s.root == nil {
		return "tree is empty"
	}
	out := ""
	s.output(s.root, "", false, &out)
	return out
}

func (s *OrderedMerkleTree) output(node *node, prefix string, isTail bool, str *string) {
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
	*str += fmt.Sprintf("%X\n", node.data.Hash)
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

// EvalMerklePath returns root hash calculated from the given hash chain, or zerohash if chain is empty
func EvalMerklePath(merklePath []*Data, value []byte, hashAlgorithm crypto.Hash) []byte {
	if len(merklePath) == 0 {
		return make([]byte, hashAlgorithm.Size())
	}
	hasher := hashAlgorithm.New()
	var h []byte
	for i, item := range merklePath {
		if i == 0 {
			h = computeLeafTreeHash(item, hasher)
		} else {
			hasher.Write([]byte{0})
			hasher.Write(item.Val)
			if bytes.Compare(value, item.Val) <= 0 {
				hasher.Write(h)
				hasher.Write(item.Hash)
			} else {
				hasher.Write(item.Hash)
				hasher.Write(h)
			}
			h = hasher.Sum(nil)
		}
		hasher.Reset()
	}
	return h
}
