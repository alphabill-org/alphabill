package state

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

	// UnitData is generic datatype for the tree. Is connected to SummaryValue through the Value function.
	UnitData interface {
		// AddToHasher adds the value of summary value to the hasher
		AddToHasher(hasher hash.Hash)
		// Value returns the SummaryValue of this single UnitData.
		Value() SummaryValue
	}

	// SummaryValue is different from UnitData. It is derived from UnitData with UnitData.Value function.
	SummaryValue interface {
		// AddToHasher adds the value of summary value to the hasher
		AddToHasher(hasher hash.Hash)
		// Concatenate calculates new SummaryValue by concatenating this, left and right.
		Concatenate(left, right SummaryValue) SummaryValue
	}

	// A tree that hold any type of units
	unitTree struct {
		hashAlgorithm crypto.Hash
		shardId       []byte
		roundNumber   uint64
		root          *Node
		// TODO add trust base: https://guardtime.atlassian.net/browse/AB-91
	}

	Unit struct {
		Bearer    Predicate // The owner predicate of the Item/Node
		Data      UnitData  // The Data part of the Item/Node
		StateHash []byte    // The hash of the transaction and previous transaction. See spec 'Unit Ledger' for details.
	}

	Node struct {
		ID           *uint256.Int // The identifier of the Item/Node
		Content      *Unit        // Content that moves together with Node
		SummaryValue SummaryValue // Calculated SummaryValue of the Item/Node
		Hash         []byte       // The hash of the node inside the tree. See spec 'State Invariants and Parameters' for details.
		Parent       *Node        // The parent node
		Children     [2]*Node     // The children (0 - left, 1 - right)
		recompute    bool         // true if node content (hash or summary value) needs recalculation
		balance      int          // AVL specific balance factor
	}
)

// New creates new UnitsTree
func New(hashAlgorithm crypto.Hash) (*unitTree, error) {
	if hashAlgorithm != crypto.SHA256 && hashAlgorithm != crypto.SHA512 {
		return nil, errors.New(errStrInvalidHashAlgorithm)
	}
	return &unitTree{
		hashAlgorithm: hashAlgorithm,
	}, nil
}

func (u *unitTree) Delete(id *uint256.Int) error {
	//TODO done in https://guardtime.atlassian.net/browse/AB-47
	panic("implement me")
}

func (u *unitTree) Get(id *uint256.Int) (*Unit, error) {
	node, exists := getNode(u.root, id)
	if !exists {
		return nil, errors.Errorf(errStrItemDoesntExist, id)
	}
	return node.Content, nil
}

// Set sets the item bearer and data. It's up to the caller to make sure the UnitData implementation supports the all data implementations inserted into the tree.
func (u *unitTree) Set(id *uint256.Int, owner Predicate, data UnitData, stateHash []byte) error {
	put(id, &Unit{
		Bearer:    owner,
		Data:      data,
		StateHash: stateHash},
		nil, &u.root)
	return nil
}

func (u *unitTree) SetOwner(id *uint256.Int, owner Predicate, stateHash []byte) error {
	node, exists := getNode(u.root, id)
	if !exists {
		return errors.Errorf(errStrItemDoesntExist, id)
	}
	put(id, &Unit{
		Bearer:    owner,
		Data:      node.Content.Data,
		StateHash: stateHash}, nil, &u.root)
	return nil
}

func (u *unitTree) SetData(id *uint256.Int, data UnitData, stateHash []byte) error {
	node, exists := getNode(u.root, id)
	if !exists {
		return errors.Errorf(errStrItemDoesntExist, id)
	}
	put(id, &Unit{
		Bearer:    node.Content.Bearer,
		Data:      data,
		StateHash: stateHash}, nil, &u.root)
	return nil
}

func (u *unitTree) Exists(id *uint256.Int) (bool, error) {
	_, exists := getNode(u.root, id)
	return exists, nil
}

func (u *unitTree) GetRootHash() []byte {
	if u.root == nil {
		return nil
	}
	u.recompute(u.root, u.hashAlgorithm.New())
	return u.root.Hash
}

func (u *unitTree) GetSummaryValue() SummaryValue {
	if u.root == nil {
		return nil
	}
	u.recompute(u.root, u.hashAlgorithm.New())
	return u.root.SummaryValue
}

func (u *unitTree) recompute(n *Node, hasher hash.Hash) {
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
		n.SummaryValue = n.Content.Data.Value().Concatenate(leftTotalValue, rightTotalValue)

		hasher.Reset()
		n.addToHasher(hasher)
		n.Hash = hasher.Sum(nil)
		hasher.Reset()
		n.recompute = false
	}
}

// addToHasher calculates the hash of the node. It also resets the hasher while doing so.
// H(ID, H(StateHash, H(ID, Bearer, UnitData)), self.SummaryValue, leftChild.hash, leftChild.SummaryValue, rightChild.hash, rightChild.summaryValue)
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

	idBytes := n.ID.Bytes32()

	// Sub hash H(ID, Bearer, UnitData)
	hasher.Reset()
	hasher.Write(idBytes[:])
	hasher.Write(n.Content.Bearer)
	n.Content.Data.AddToHasher(hasher)
	hashSub1 := hasher.Sum(nil)

	// Sub hash H(StateHash, subHash1)
	hasher.Reset()
	hasher.Write(n.Content.StateHash)
	hasher.Write(hashSub1)
	hashSub2 := hasher.Sum(nil)

	// Main hash
	hasher.Reset()
	hasher.Write(idBytes[:])
	hasher.Write(hashSub2)
	n.SummaryValue.AddToHasher(hasher)

	hasher.Write(leftHash)
	if left != nil {
		left.SummaryValue.AddToHasher(hasher)
	}
	hasher.Write(rightHash)
	if right != nil {
		right.SummaryValue.AddToHasher(hasher)
	}
}

// String returns a string representation of the node
func (n *Node) String() string {
	m := fmt.Sprintf("ID=%v, ", n.ID.Bytes32())
	if n.recompute {
		m = m + "*"
	}
	m = m + fmt.Sprintf("value=%v, total=%v, bearer=%X, stateHash=%X,",
		n.Content.Data, n.SummaryValue, n.Content.Bearer, n.Content.StateHash)

	if n.Hash != nil {
		m = m + fmt.Sprintf("hash=%X", n.Hash)
	}

	return m
}
