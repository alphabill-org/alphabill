package imt

import (
	"bytes"
	"crypto"
	"fmt"
	"hash"
)

const (
	Leaf byte = 0
	Node byte = 1
)

type (
	IMT struct {
		root       *node
		dataLength int // number of leaves
	}

	// Data for calculating hash of leaf data
	Data interface {
		AddToHasher(hasher hash.Hash)
	}

	// LeafData index tree leaf. NB! Index tree leaves must be sorted lexicographically in strictly ascending order
	LeafData struct {
		Index []byte
		Data  Data
	}
	// PathItem helper struct for proof extraction, contains Hash and Index of node
	PathItem struct {
		Index []byte
		Hash  []byte
	}

	pair struct {
		index    []byte
		dataHash []byte
	}
	node struct {
		left  *node
		right *node
		hash  []byte
		index []byte
	}
)

func (n *node) isLeaf() bool {
	if n.left == nil && n.right == nil {
		return true
	}
	return false
}

// New creates a new indexed Merkle Tree.
func New(hashAlgorithm crypto.Hash, leaves []LeafData) (*IMT, error) {
	if len(leaves) == 0 {
		return &IMT{root: nil, dataLength: 0}, nil
	}
	// validate order - perhaps could just sort here instead?
	for i := len(leaves) - 1; i > 0; i-- {
		if bytes.Compare(leaves[i].Index, leaves[i-1].Index) != 1 {
			return nil, fmt.Errorf("data not sorted by index in not strictly ascending order")
		}
	}
	hasher := hashAlgorithm.New()
	// calculate data hash for leaves
	pairs := make([]pair, len(leaves))
	for i, l := range leaves {
		l.Data.AddToHasher(hasher)
		pairs[i] = pair{index: l.Index, dataHash: hasher.Sum(nil)}
		hasher.Reset()
	}
	return &IMT{root: createMerkleTree(pairs, hasher), dataLength: len(pairs)}, nil
}

// IndexTreeOutput calculates the output hash of the index Merkle tree hash chain.
func IndexTreeOutput(merklePath []*PathItem, index []byte, data Data, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	// calculate data hash
	data.AddToHasher(hasher)
	dataHash := hasher.Sum(nil)
	hasher.Reset()
	// calculate leaf hash
	hasher.Write([]byte{Leaf})
	hasher.Write(index)
	hasher.Write(dataHash)
	h := hasher.Sum(nil)
	hasher.Reset()
	// follow hash chain
	for _, item := range merklePath {
		if bytes.Compare(index, item.Index) == 1 {
			// index > item.Index is bigger - left link
			hasher.Write([]byte{Node})
			hasher.Write(item.Index)
			hasher.Write(item.Hash)
			hasher.Write(h)
		} else {
			// index <= item.Index is smaller or equal right link
			hasher.Write([]byte{Node})
			hasher.Write(item.Index)
			hasher.Write(h)
			hasher.Write(item.Hash)
		}
		h = hasher.Sum(nil)
		hasher.Reset()
	}
	return h
}

// GetRootHash returns the root Hash of the indexed Merkle tree.
func (s *IMT) GetRootHash() []byte {
	if s.root == nil {
		return nil
	}
	return s.root.hash
}

// GetMerklePath extracts the indexed merkle hash chain from the given leaf to root.
func (s *IMT) GetMerklePath(leafIdx []byte) ([]*PathItem, error) {
	if s.root == nil {
		return nil, fmt.Errorf("tree empty")
	}
	var z []*PathItem
	curr := s.root
	for !curr.isLeaf() {
		if bytes.Compare(leafIdx, curr.index) == 1 {
			z = append([]*PathItem{{Index: curr.index, Hash: curr.left.hash}}, z...)
			curr = curr.right
		} else { // smaller or equal index
			z = append([]*PathItem{{Index: curr.index, Hash: curr.right.hash}}, z...)
			curr = curr.left
		}
	}
	return z, nil
}

// PrettyPrint returns a human-readable string representation of the indexed Merkle tree.
func (s *IMT) PrettyPrint() string {
	if s == nil || s.root == nil {
		return "────┤ empty"
	}
	out := ""
	s.output(s.root, "", false, &out)
	return out
}

func createMerkleTree(pairs []pair, hasher hash.Hash) *node {
	if len(pairs) == 0 {
		return &node{hash: make([]byte, hasher.Size())}
	}
	if len(pairs) == 1 {
		hasher.Reset()
		hasher.Write([]byte{Leaf})
		hasher.Write(pairs[0].index)
		hasher.Write(pairs[0].dataHash)
		return &node{index: pairs[0].index, hash: hasher.Sum(nil)}
	}
	m := (len(pairs) + 1) / 2
	leftSub := pairs[:m]
	rightSub := pairs[m:]
	left := createMerkleTree(leftSub, hasher)
	right := createMerkleTree(rightSub, hasher)
	hasher.Reset()
	hasher.Write([]byte{Node})
	hasher.Write(leftSub[len(leftSub)-1].index)
	hasher.Write(left.hash)
	hasher.Write(right.hash)
	return &node{index: leftSub[len(leftSub)-1].index, left: left, right: right, hash: hasher.Sum(nil)}
}

func (s *IMT) output(node *node, prefix string, isTail bool, str *string) {
	if node.right != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "│\t"
		} else {
			newPrefix += "\t"
		}
		s.output(node.right, newPrefix, false, str)
	}
	*str += prefix
	if isTail {
		*str += "└──"
	} else {
		*str += "┌──"
	}
	*str += fmt.Sprintf("i: %x, %X\n", node.index, node.hash)
	if node.left != nil {
		newPrefix := prefix
		if isTail {
			newPrefix += "\t"
		} else {
			newPrefix += "│\t"
		}
		s.output(node.left, newPrefix, true, str)
	}
}
