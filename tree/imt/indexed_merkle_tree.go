package imt

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
)

var ErrIndexOutOfBounds = errors.New("index out of bounds")

type NodeType byte

const (
	Leaf NodeType = 0
	Node NodeType = 1
)

type (
	MerkleTree struct {
		root       *node
		dataLength int // number of leaves
	}

	Pair struct {
		Index []byte
		Data  Data
	}

	Data interface {
		Hash(hashAlgorithm crypto.Hash) []byte
	}

	// PathItem helper struct for proof extraction, contains Hash and Direction from parent node
	PathItem struct {
		Index []byte
		Hash  []byte
	}

	node struct {
		left  *node
		right *node
		hash  []byte
		index []byte
		nType NodeType
	}
)

func (p *Pair) Hash(hashAlgorithm crypto.Hash, t NodeType) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write([]byte{byte(t)})
	hasher.Write(p.Index)
	hasher.Write(p.Data.Hash(hashAlgorithm))
	return hasher.Sum(nil)
}

// New creates a new canonical Merkle Tree.
func New(hashAlgorithm crypto.Hash, data []Pair) *MerkleTree {
	if len(data) == 0 {
		return &MerkleTree{root: nil, dataLength: 0}
	}
	return &MerkleTree{root: createMerkleTree(data, hashAlgorithm), dataLength: len(data)}
}

// EvalMerklePath returns root hash calculated from the given leaf and path items
func EvalMerklePath(merklePath []*PathItem, leaf Pair, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	h := leaf.Hash(hashAlgorithm, Leaf)
	if len(merklePath) == 0 {
		return nil
	}
	merklePath = merklePath[1:]
	for _, item := range merklePath {
		if bytes.Compare(leaf.Index, item.Index) == 1 {
			// index > item.Index is bigger - left link
			hasher.Write([]byte{byte(Node)})
			hasher.Write(item.Index)
			hasher.Write(item.Hash)
			hasher.Write(h)
		} else {
			// index <= item.Index is smaller or equal right link
			hasher.Write([]byte{byte(Node)})
			hasher.Write(item.Index)
			hasher.Write(h)
			hasher.Write(item.Hash)
		}
		h = hasher.Sum(nil)
		hasher.Reset()
	}
	return h
}

// IndexTreeOutput calculates the output hash of the chain.
func IndexTreeOutput(merklePath []*PathItem, index []byte, hashAlgorithm crypto.Hash) []byte {
	if len(merklePath) == 0 {
		return nil
	}
	h := merklePath[0].Hash

	if len(merklePath) == 1 {
		return h
	}
	hasher := hashAlgorithm.New()

	merklePath = merklePath[1:]
	for _, item := range merklePath {
		if bytes.Compare(index, item.Index) == 1 {
			// index > item.Index is bigger - left link
			hasher.Write([]byte{byte(Node)})
			hasher.Write(item.Index)
			hasher.Write(item.Hash)
			hasher.Write(h)
		} else {
			// index <= item.Index is smaller or equal right link
			hasher.Write([]byte{byte(Node)})
			hasher.Write(item.Index)
			hasher.Write(h)
			hasher.Write(item.Hash)
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
func (s *MerkleTree) GetMerklePath(leafIdx []byte) ([]*PathItem, error) {
	if s.root == nil {
		return nil, fmt.Errorf("tree empty")
	}
	if s.root.nType == Leaf {
		return []*PathItem{{Index: s.root.index, Hash: s.root.hash}}, nil
	}
	var z []*PathItem
	curr := s.root
	for {
		if bytes.Compare(leafIdx, curr.index) == 1 {
			z = append([]*PathItem{{Index: curr.index, Hash: curr.left.hash}}, z...)
			curr = curr.right
		} else { // smaller or equal index
			z = append([]*PathItem{{Index: curr.index, Hash: curr.right.hash}}, z...)
			curr = curr.left
		}
		// leaf is the last node
		if curr.nType == Leaf {
			z = append([]*PathItem{{Index: curr.index, Hash: curr.hash}}, z...)
			break
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

func createMerkleTree(data []Pair, hashAlgorithm crypto.Hash) *node {
	if len(data) == 0 {
		return &node{hash: make([]byte, hashAlgorithm.Size())}
	}
	if len(data) == 1 {
		return &node{index: data[0].Index, nType: Leaf, hash: data[0].Hash(hashAlgorithm, Leaf)}
	}
	m := (len(data) + 1) / 2
	leftSub := data[:m]
	rightSub := data[m:]
	hasher := hashAlgorithm.New()
	left := createMerkleTree(leftSub, hashAlgorithm)
	right := createMerkleTree(rightSub, hashAlgorithm)
	hasher.Write([]byte{byte(Node)})
	hasher.Write(leftSub[len(leftSub)-1].Index)
	hasher.Write(left.hash)
	hasher.Write(right.hash)
	return &node{index: leftSub[len(leftSub)-1].Index, nType: Node, left: left, right: right, hash: hasher.Sum(nil)}
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
	*str += fmt.Sprintf("i: %x, %X\n", node.index, node.hash)
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
