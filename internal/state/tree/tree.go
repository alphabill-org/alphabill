package tree

import (
	"crypto"
	"fmt"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"github.com/holiman/uint256"
)

const (
	errStrItemDoesntExist      = "item %d does not exist"
	errStrInvalidHashAlgorithm = "invalid hash algorithm"
)

type (
	Predicate []byte

	// Data is generic datatype for the tree. Is connected to SummaryValue through the Value function.
	Data interface {
		// AddToHasher adds the value of summary value to the hasher
		AddToHasher(hasher hash.Hash)
		// Value returns the SummaryValue of this single Data.
		Value() SummaryValue
	}

	// SummaryValue is different from Data. It is derived from Data with Data.Value function.
	SummaryValue interface {
		// AddToHasher adds the value of summary value to the hasher
		AddToHasher(hasher hash.Hash)
		// Concatenate calculates new SummaryValue by concatenating this, left and right.
		Concatenate(left, right SummaryValue) SummaryValue
	}

	// A tree that hold any type of units
	unitsTree struct {
		hashAlgorithm crypto.Hash
		shardId       []byte
		roundNumber   uint64
		root          *Node
		// TODO add trust base: https://guardtime.atlassian.net/browse/AB-91
	}

	Node struct {
		ID           *uint256.Int // The identifier of the Item/Node
		Bearer       Predicate    // The owner predicate of the Item/Node
		Data         Data         // The Data part of the Item/Node
		StateHash    []byte       // The hash of the transaction and previous transaction. See spec 'Unit Ledger' for details.
		SummaryValue SummaryValue // Calculated SummaryValue of the Item/Node
		Hash         []byte       // The hash of the node inside the tree. See spec 'State Invariants and Parameters' for details.
		Parent       *Node        // The parent node
		Children     [2]*Node     // The children (0 - left, 1 - right)
		recompute    bool         // true if node content (hash or summary value) needs recalculation
		balance      int          // AVL specific balance factor
	}
)

// New creates new UnitsTree
func New(hashAlgorithm crypto.Hash) (*unitsTree, error) {
	if hashAlgorithm != crypto.SHA256 && hashAlgorithm != crypto.SHA512 {
		return nil, errors.New(errStrInvalidHashAlgorithm)
	}
	return &unitsTree{
		hashAlgorithm: hashAlgorithm,
	}, nil
}

func (u *unitsTree) Delete(id *uint256.Int) error {
	//TODO done in https://guardtime.atlassian.net/browse/AB-47
	panic("implement me")
}

func (u *unitsTree) Get(id *uint256.Int) (owner Predicate, data Data, err error) {
	node, exists := getNode(u.root, id)
	if !exists {
		return nil, nil, errors.Errorf(errStrItemDoesntExist, id)
	}
	return node.Bearer, node.Data, nil
}

// Set sets the item bearer and data. It's up to the caller to make sure the Data implementation supports the all data implementations inserted into the tree.
func (u *unitsTree) Set(id *uint256.Int, bearer Predicate, data Data) error {
	put(id, data, bearer, nil, &u.root)
	return nil
}

func (u *unitsTree) SetOwner(id *uint256.Int, owner Predicate) error {
	node, exists := getNode(u.root, id)
	if !exists {
		return errors.Errorf(errStrItemDoesntExist, id)
	}
	put(id, node.Data, owner, nil, &u.root)
	return nil
}

func (u *unitsTree) SetData(id *uint256.Int, data Data) error {
	node, exists := getNode(u.root, id)
	if !exists {
		return errors.Errorf(errStrItemDoesntExist, id)
	}
	put(id, data, node.Bearer, nil, &u.root)
	return nil
}

func (u *unitsTree) Exists(id *uint256.Int) (bool, error) {
	_, exists := getNode(u.root, id)
	return exists, nil
}

func (u *unitsTree) GetRootHash() []byte {
	if u.root == nil {
		return nil
	}
	u.recompute(u.root, u.hashAlgorithm.New())
	return u.root.Hash
}

func (u *unitsTree) GetSummaryValue() SummaryValue {
	if u.root == nil {
		return nil
	}
	u.recompute(u.root, u.hashAlgorithm.New())
	return u.root.SummaryValue
}

func (u *unitsTree) recompute(n *Node, hasher hash.Hash) {
	if n.recompute {
		var leftTotalValue SummaryValue
		var rightTotalValue SummaryValue

		var left = n.Children[0]
		var right = n.Children[1]
		if left != nil {
			u.recompute(left, hasher)
			leftTotalValue = left.SummaryValue
		}
		if right != nil {
			u.recompute(right, hasher)
			rightTotalValue = right.SummaryValue
		}
		n.SummaryValue = n.Data.Value().Concatenate(leftTotalValue, rightTotalValue)

		hasher.Reset()
		n.addToHasher(hasher)
		n.Hash = hasher.Sum(nil)
		hasher.Reset()
		n.recompute = false
	}
}

// ID, H(StateHash, H(ID, Bearer, Data)), self.SummaryValue, leftChild.hash, leftChild.SummaryValue, rightChild.hash, rightChild.summaryValue)
func (n *Node) addToHasher(hasher hash.Hash) {
	leftHash := make([]byte, hasher.Size())
	rightHash := make([]byte, hasher.Size())
	var left = n.Children[0]
	var right = n.Children[1]
	if left != nil {
		leftHash = left.Hash
	}
	if right != nil {
		rightHash = right.Hash
	}
	hasher.Write(leftHash)
	hasher.Write(rightHash)
}

// String returns a string representation of the node
func (n *Node) String() string {
	m := fmt.Sprintf("ID=%v, ", n.ID.Bytes32())
	if n.recompute {
		m = m + "*"
	}
	m = m + fmt.Sprintf("value=%v, total=%v, bearer=%X, ",
		n.Data, n.SummaryValue, n.Bearer)

	if n.Hash != nil {
		m = m + fmt.Sprintf("hash=%X", n.Hash)
	}

	return m
}
