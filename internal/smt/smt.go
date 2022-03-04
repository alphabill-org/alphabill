package smt

import (
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

const (
	zero       int = 0
	one        int = 1
	seven      int = 7
	bitsInByte int = 8
)

var ErrInvalidKeyLength = errors.New("invalid key length")

type (
	SMT struct {
		keyLength int
		hasher    hash.Hash
		root      *node
	}

	Data interface {
		Key(keyLength int) []byte
		Value() []byte
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
	root, err := createSMT(&node{}, zero, keyLength*bitsInByte, data, hasher)
	if err != nil {
		return nil, err
	}
	return &SMT{
		keyLength: keyLength,
		hasher:    hasher,
		root:      root,
	}, nil
}

// GetAuthPath returns authentication path for given key.
func (s *SMT) GetAuthPath(key []byte) ([][]byte, error) {
	if len(key) != s.keyLength {
		return nil, ErrInvalidKeyLength
	}
	treeHeight := s.keyLength * bitsInByte
	result := make([][]byte, treeHeight)
	node := s.root
	for i := 0; i < treeHeight-1; i++ {
		if node == nil {
			result[treeHeight-i-1] = make([]byte, s.hasher.BlockSize())
			continue
		}
		var pathItem []byte
		if isBitSet(key, i) {
			if node.left == nil {
				pathItem = make([]byte, s.hasher.BlockSize())
			} else {
				pathItem = node.left.hash
			}
			node = node.right
		} else {
			if node.right == nil {
				pathItem = make([]byte, s.hasher.BlockSize())
			} else {
				pathItem = node.right.hash
			}
			node = node.left
		}
		result[treeHeight-i-1] = pathItem
	}
	if node == nil {
		result[0] = make([]byte, s.hasher.BlockSize())
		return result, nil
	}
	result[0] = node.hash
	return result, nil
}

func createSMT(p *node, position int, maxPositionSize int, data []Data, hasher hash.Hash) (*node, error) {
	if len(data) == 0 {
		// Zero hash
		p.hash = make([]byte, hasher.BlockSize())
		return p, nil
	}
	if position == maxPositionSize-1 {
		// leaf
		d := data[0]
		hasher.Write(d.Value())
		p.hash = hasher.Sum(nil)
		p.data = d
		hasher.Reset()
		return p, nil
	}
	keySize := maxPositionSize / bitsInByte
	// inner node
	var leftData []Data
	var rightData []Data
	for _, id := range data {
		key := id.Key(keySize)
		if len(key) != keySize {
			return nil, ErrInvalidKeyLength
		}
		if !isBitSet(key, position) {
			leftData = append(leftData, id)
		} else {
			rightData = append(rightData, id)
		}
	}
	position++
	var err error
	p.left, err = createSMT(&node{}, position, maxPositionSize, leftData, hasher)
	if err != nil {
		return nil, err
	}
	p.right, err = createSMT(&node{}, position, maxPositionSize, rightData, hasher)
	if err != nil {
		return nil, err
	}
	hasher.Write(p.left.hash)
	hasher.Write(p.right.hash)
	p.hash = hasher.Sum(nil)
	hasher.Reset()
	return p, nil
}

func isBitSet(bytes []byte, bitPosition int) bool {
	byteIndex := bitPosition / bitsInByte
	bitIndexInByte := bitPosition % bitsInByte
	return bytes[byteIndex]&byte(one<<(seven-bitIndexInByte)) != byte(zero)
}
