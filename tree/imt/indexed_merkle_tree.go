package imt

import (
	"bytes"
	"crypto"
	"fmt"
	"hash"
)

const (
	Node byte = 0
	Leaf byte = 1
)

type (
	IMT struct {
		root       *node
		dataLength int // number of leaves
	}

	// LeafData indexed tree leaf.
	// NB!: indexed tree leaves must be sorted lexicographically by key in strict order k1 < k2 < ... kn
	LeafData interface {
		Key() []byte
		AddToHasher(hasher hash.Hash)
	}
	// PathItem helper struct for proof extraction, contains Hash and Index of node
	PathItem struct {
		Key  []byte
		Hash []byte
	}

	pair struct {
		key      []byte
		dataHash []byte
	}
	node struct {
		left     *node
		right    *node
		hash     []byte
		dataHash []byte // only leaf nodes have data hash
		k        []byte
	}
)

func (n *node) isLeaf() bool {
	if n.left == nil && n.right == nil {
		return true
	}
	return false
}

// New creates a new indexed Merkle tree.
func New(hashAlgorithm crypto.Hash, leaves []LeafData) (*IMT, error) {
	if len(leaves) == 0 {
		return &IMT{root: nil, dataLength: 0}, nil
	}
	// validate order
	for i := len(leaves) - 1; i > 0; i-- {
		if bytes.Compare(leaves[i].Key(), leaves[i-1].Key()) != 1 {
			return nil, fmt.Errorf("data not sorted by key in not strictly ascending order")
		}
	}
	hasher := hashAlgorithm.New()
	// calculate data hash for leaves
	pairs := make([]pair, len(leaves))
	for i, l := range leaves {
		l.AddToHasher(hasher)
		pairs[i] = pair{key: l.Key(), dataHash: hasher.Sum(nil)}
		hasher.Reset()
	}
	return &IMT{root: createMerkleTree(pairs, hasher), dataLength: len(pairs)}, nil
}

// IndexTreeOutput calculates the output hash of the index Merkle tree hash chain from hash chain, key and data hash.
func IndexTreeOutput(merklePath []*PathItem, key []byte, hashAlgorithm crypto.Hash) []byte {
	if len(merklePath) == 0 {
		return nil
	}
	leaf, merklePath := merklePath[0], merklePath[1:]

	hasher := hashAlgorithm.New()
	hasher.Write([]byte{Leaf})
	hasher.Write(leaf.Key)
	hasher.Write(leaf.Hash)
	h := hasher.Sum(nil)
	hasher.Reset()
	// follow hash chain
	for _, item := range merklePath {
		if bytes.Compare(key, item.Key) == 1 {
			// key > item.Key is bigger - left link
			hasher.Write([]byte{Node})
			hasher.Write(item.Key)
			hasher.Write(item.Hash)
			hasher.Write(h)
		} else {
			// key <= item.Key is smaller or equal right link
			hasher.Write([]byte{Node})
			hasher.Write(item.Key)
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

// GetMerklePath extracts the indexed merkle hash chain from the given leaf key
// to root. A hash chain is always returned. If the key is not present, a chain
// is returned from where the key is supposed to be.
// This can be used to prove that the key was not present.
func (s *IMT) GetMerklePath(key []byte) ([]*PathItem, error) {
	if s.root == nil {
		return nil, fmt.Errorf("tree empty")
	}
	var z []*PathItem
	curr := s.root
	for !curr.isLeaf() {
		if bytes.Compare(key, curr.k) == 1 {
			z = append([]*PathItem{{Key: curr.k, Hash: curr.left.hash}}, z...)
			curr = curr.right
		} else { // smaller or equal key
			z = append([]*PathItem{{Key: curr.k, Hash: curr.right.hash}}, z...)
			curr = curr.left
		}
	}
	// append leaf
	z = append([]*PathItem{{Key: curr.k, Hash: curr.dataHash}}, z...)
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
		hasher.Write(pairs[0].key)
		hasher.Write(pairs[0].dataHash)
		return &node{k: pairs[0].key, dataHash: pairs[0].dataHash, hash: hasher.Sum(nil)}
	}
	m := (len(pairs) + 1) / 2
	leftSub := pairs[:m]
	rightSub := pairs[m:]
	left := createMerkleTree(leftSub, hasher)
	right := createMerkleTree(rightSub, hasher)
	hasher.Reset()
	hasher.Write([]byte{Node})
	hasher.Write(leftSub[len(leftSub)-1].key)
	hasher.Write(left.hash)
	hasher.Write(right.hash)
	return &node{k: leftSub[len(leftSub)-1].key, left: left, right: right, hash: hasher.Sum(nil)}
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
	*str += fmt.Sprintf("k: %x, %X\n", node.k, node.hash)
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
