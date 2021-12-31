package tree

import (
	"fmt"

	"github.com/holiman/uint256"
)

type (
	Predicate    []byte
	Data         interface{}
	SummaryValue interface{}

	// A tree that hold any type of units
	unitsTree struct {
		shardId     []byte
		roundNumber uint64
		root        *Node
		// TODO trust base? Probably a function
	}

	Node struct {
		// TODO add comments
		ID           *uint256.Int
		Bearer       Predicate
		Data         Data
		StateHash    []byte
		SummaryValue SummaryValue
		Hash         []byte
		Parent       *Node
		Children     [2]*Node
		recompute    bool // true if node content (hash, value, total value, etc) needs recalculation
		balance      int  // balance factor
	}
)

func New() *unitsTree {
	return &unitsTree{}
}

func (u unitsTree) Delete(id *uint256.Int) error {
	//TODO implement me
	panic("implement me")
}

func (u unitsTree) Get(id *uint256.Int) (owner Predicate, data Data, err error) {
	//TODO implement me
	panic("implement me")
}

func (u unitsTree) Set(id *uint256.Int, bearer Predicate, data Data) error {
	put(id, data, bearer, nil, &u.root)
	return nil
}

func (u unitsTree) SetOwner(id *uint256.Int, owner Predicate) error {
	//TODO implement me
	panic("implement me")
}

func (u unitsTree) SetData(id *uint256.Int, data Data) error {
	//TODO implement me
	panic("implement me")
}

func (u unitsTree) Exists(id *uint256.Int) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (u unitsTree) GetRootHash() []byte {
	return nil
}

func (u unitsTree) GetSummaryValue() SummaryValue {
	return nil
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
