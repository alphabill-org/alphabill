package smt

import (
	"crypto"
	"fmt"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var ErrInvalidKeyLength = errors.New("invalid key length")

type (
	SMT struct {
		keyLength int
		hasher    hash.Hash
		root      *node
		zeroHash  []byte
	}

	Data interface {
		Key() []byte
		AddToHasher(hasher hash.Hash)
	}

	node struct {
		left  *node
		right *node
		hash  []byte
		data  Data // Data is present in leaf nodes.
	}
)

// New creates a new sparse merkle tree.
func New(hasher hash.Hash, keyLength int, data []Data) (*SMT, error) {
	zeroHash := make([]byte, hasher.Size())
	root, err := createSMT(&node{}, 0, keyLength*8, data, hasher, zeroHash)
	if err != nil {
		return nil, err
	}
	return &SMT{
		keyLength: keyLength,
		hasher:    hasher,
		root:      root,
		zeroHash:  zeroHash,
	}, nil
}

// GetRootHash returns the root hash of the SMT.
func (s *SMT) GetRootHash() []byte {
	return s.root.hash
}

// GetAuthPath returns authentication path and leaf node data for given key.
func (s *SMT) GetAuthPath(key []byte) ([][]byte, Data, error) {
	if len(key) != s.keyLength {
		return nil, nil, ErrInvalidKeyLength
	}
	treeHeight := s.keyLength * 8
	proofLength := treeHeight - 1
	result := make([][]byte, proofLength)
	node := s.root
	// skips leaf
	for i := 0; i < treeHeight-1; i++ {
		if node == nil {
			result[proofLength-i-1] = s.zeroHash
			continue
		}
		var pathItem []byte
		if util.IsBitSet(key, i) {
			if node.left == nil {
				pathItem = s.zeroHash
			} else {
				pathItem = node.left.hash
			}
			node = node.right
		} else {
			if node.right == nil {
				pathItem = s.zeroHash
			} else {
				pathItem = node.right.hash
			}
			node = node.left
		}
		result[proofLength-i-1] = pathItem
	}
	if node == nil {
		return result, nil, nil
	}
	return result, node.data, nil
}

func CalculatePathRoot(path [][]byte, leafHash []byte, key []byte, hashAlgorithm crypto.Hash) ([]byte, error) {
	if key == nil {
		return nil, ErrInvalidKeyLength
	}
	if len(path) != len(key)*8-1 {
		return nil, errors.Errorf("invalid path/key combination: path length=%v, key length=%v", len(path), len(key))
	}
	if len(leafHash) != hashAlgorithm.Size() {
		return nil, errors.Errorf("invalid leaf hash length: leaf length=%v, hash length=%v", len(leafHash), hashAlgorithm.Size())
	}
	hasher := hashAlgorithm.New()
	h := leafHash
	pathLength := len(path)
	for i := 0; i < pathLength; i++ {
		pathItem := path[i]
		if util.IsBitSet(key, pathLength-1-i) {
			hasher.Write(pathItem)
			hasher.Write(h)
		} else {
			hasher.Write(h)
			hasher.Write(pathItem)
		}
		h = hasher.Sum(nil)
		hasher.Reset()
	}
	return h, nil
}

func (s *SMT) PrettyPrint() string {
	if s.root == nil {
		return "tree is empty"
	}
	out := ""
	s.output(s.root, "", false, &out)
	return out
}

// output is rmaTree inner method for producing debugging
func (s *SMT) output(node *node, prefix string, isTail bool, str *string) {
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

func createSMT(p *node, position int, maxPositionSize int, data []Data, hasher hash.Hash, zeroHash []byte) (*node, error) {
	if len(data) == 0 {
		// Zero hash
		p.hash = zeroHash
		return p, nil
	}
	if position == maxPositionSize-1 {
		// leaf
		d := data[0]
		d.AddToHasher(hasher)
		p.hash = hasher.Sum(nil)
		p.data = d
		hasher.Reset()
		return p, nil
	}
	keySize := maxPositionSize / 8
	// inner node
	var leftData []Data
	var rightData []Data
	for _, id := range data {
		key := id.Key()
		if len(key) != keySize {
			return nil, ErrInvalidKeyLength
		}
		if !util.IsBitSet(key, position) {
			leftData = append(leftData, id)
		} else {
			rightData = append(rightData, id)
		}
	}
	position++
	var err error
	p.left, err = createSMT(&node{}, position, maxPositionSize, leftData, hasher, zeroHash)
	if err != nil {
		return nil, err
	}
	p.right, err = createSMT(&node{}, position, maxPositionSize, rightData, hasher, zeroHash)
	if err != nil {
		return nil, err
	}
	hasher.Write(p.left.hash)
	hasher.Write(p.right.hash)
	p.hash = hasher.Sum(nil)
	hasher.Reset()
	return p, nil
}
