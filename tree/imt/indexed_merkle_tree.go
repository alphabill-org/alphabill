package imt

import (
	"bytes"
	"crypto"
	"fmt"
	"hash"
)

const (
	tagNode byte = 0
	tagLeaf byte = 1
)

type (
	Tree struct {
		root       *node
		dataLength int // number of leaves
	}

	// LeafData indexed tree leaf.
	// NB!: indexed tree leaves must be sorted lexicographically by key in strict order k1 < k2 < ... kn
	LeafData interface {
		Key() []byte
		AddToHasher(hasher hash.Hash)
	}
	// PathItem helper struct for proof extraction, contains Hash and Key of node
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
		key      []byte
	}
)

func (n *node) isLeaf() bool {
	return n.left == nil && n.right == nil
}

// New creates a new indexed Merkle tree.
func New(hashAlgorithm crypto.Hash, leaves []LeafData) (*Tree, error) {
	if len(leaves) == 0 {
		return &Tree{root: nil, dataLength: 0}, nil
	}
	// validate order
	for i := len(leaves) - 1; i > 0; i-- {
		if bytes.Compare(leaves[i].Key(), leaves[i-1].Key()) != 1 {
			return nil, fmt.Errorf("data is not sorted by key in strictly ascending order")
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
	return &Tree{root: createMerkleTree(pairs, hasher), dataLength: len(pairs)}, nil
}

// IndexTreeOutput calculates the output hash of the index Merkle tree hash chain from hash chain, key and data hash.
func IndexTreeOutput(merklePath []*PathItem, key []byte, hashAlgorithm crypto.Hash) []byte {
	if len(merklePath) == 0 {
		return nil
	}
	leaf, merklePath := merklePath[0], merklePath[1:]

	hasher := hashAlgorithm.New()
	hasher.Write([]byte{tagLeaf})
	hasher.Write(leaf.Key)
	hasher.Write(leaf.Hash)
	h := hasher.Sum(nil)
	// follow hash chain
	for _, item := range merklePath {
		hasher.Reset()
		if bytes.Compare(key, item.Key) == 1 {
			// key > item.Key is bigger - left link
			hasher.Write([]byte{tagNode})
			hasher.Write(item.Key)
			hasher.Write(item.Hash)
			hasher.Write(h)
		} else {
			// key <= item.Key is smaller or equal right link
			hasher.Write([]byte{tagNode})
			hasher.Write(item.Key)
			hasher.Write(h)
			hasher.Write(item.Hash)
		}
		h = hasher.Sum(nil)
	}
	return h
}

// GetRootHash returns the root Hash of the indexed Merkle tree.
func (s *Tree) GetRootHash() []byte {
	if s.root == nil {
		return nil
	}
	return s.root.hash
}

// GetMerklePath extracts the indexed merkle hash chain from the given leaf key
// to root. A hash chain is always returned. If the key is not present, a chain
// is returned from where the key is supposed to be.
func (s *Tree) GetMerklePath(key []byte) ([]*PathItem, error) {
	if s.root == nil {
		return nil, fmt.Errorf("tree is empty")
	}
	var z []*PathItem
	curr := s.root
	for !curr.isLeaf() {
		if bytes.Compare(key, curr.key) == 1 {
			z = append([]*PathItem{{Key: curr.key, Hash: curr.left.hash}}, z...)
			curr = curr.right
		} else { // smaller or equal key
			z = append([]*PathItem{{Key: curr.key, Hash: curr.right.hash}}, z...)
			curr = curr.left
		}
	}
	// append leaf
	z = append([]*PathItem{{Key: curr.key, Hash: curr.dataHash}}, z...)
	return z, nil
}

func createMerkleTree(pairs []pair, hasher hash.Hash) *node {
	if len(pairs) == 1 {
		hasher.Reset()
		hasher.Write([]byte{tagLeaf})
		hasher.Write(pairs[0].key)
		hasher.Write(pairs[0].dataHash)
		return &node{key: pairs[0].key, dataHash: pairs[0].dataHash, hash: hasher.Sum(nil)}
	}
	m := (len(pairs) + 1) / 2
	leftSub := pairs[:m]
	rightSub := pairs[m:]
	left := createMerkleTree(leftSub, hasher)
	right := createMerkleTree(rightSub, hasher)
	hasher.Reset()
	hasher.Write([]byte{tagNode})
	hasher.Write(leftSub[len(leftSub)-1].key)
	hasher.Write(left.hash)
	hasher.Write(right.hash)
	return &node{key: leftSub[len(leftSub)-1].key, left: left, right: right, hash: hasher.Sum(nil)}
}

// PrettyPrint returns a human-readable string representation of the indexed Merkle tree.
func (s *Tree) PrettyPrint() string {
	if s == nil || s.root == nil {
		return "────┤ empty"
	}
	out := ""
	s.output(s.root, "", false, &out)
	return out
}

func (s *Tree) output(node *node, prefix string, isTail bool, str *string) {
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
	*str += fmt.Sprintf("key: %x, %X\n", node.key, node.hash)
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
