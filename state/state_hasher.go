package state

import (
	"crypto"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/tree/mt"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
)

// stateHasher calculates the root hash of the state tree (see "Invariants of the State Tree" chapter from the
// yellowpaper for more information).
type stateHasher struct {
	avl.PostOrderCommitTraverser[types.UnitID, VersionedUnit]
	hashAlgorithm crypto.Hash
}

func newStateHasher(hashAlgorithm crypto.Hash) *stateHasher {
	return &stateHasher{hashAlgorithm: hashAlgorithm}
}

// Traverse visits changed nodes in the state tree and recalculates a new root hash of the state tree.
// Executed when the State.Commit function is called.
func (p *stateHasher) Traverse(n *avl.Node[types.UnitID, VersionedUnit]) error {
	if n == nil || (n.Clean() && UnitV1(n.Value()).summaryCalculated) {
		return nil
	}
	var left = n.Left()
	var right = n.Right()
	if err := p.Traverse(left); err != nil {
		return err
	}
	if err := p.Traverse(right); err != nil {
		return err
	}

	unit := UnitV1(n.Value())

	// h_s - calculate state log root hash
	// Skip this step if state has been recovered from file and logsHash is already present.
	if unit.logsHash == nil {
		merkleTree, err := mt.New(p.hashAlgorithm, unit.logs)
		if err != nil {
			return err
		}
		unit.logsHash = merkleTree.GetRootHash()
	}

	unit.stateLockTx = unit.latestStateLockTx()

	// D - unit data
	unit.data = unit.latestUnitData()

	// V - calculate summary value
	leftSummary := getSubTreeSummaryValue(left)
	rightSummary := getSubTreeSummaryValue(right)
	unitDataSummaryInputValue := getSummaryValueInput(n)
	unit.subTreeSummaryValue = unitDataSummaryInputValue + leftSummary + rightSummary

	// h - subtree summary hash
	hasher := abhash.New(p.hashAlgorithm.New())
	hasher.Write(n.Key())
	hasher.Write(unit.logsHash)
	hasher.Write(unit.subTreeSummaryValue)
	hasher.Write(getSubTreeSummaryHash(left))
	hasher.Write(leftSummary)
	hasher.Write(getSubTreeSummaryHash(right))
	hasher.Write(rightSummary)

	var err error
	unit.subTreeSummaryHash, err = hasher.Sum()
	if err != nil {
		return err
	}
	unit.summaryCalculated = true
	p.SetClean(n)
	return nil
}
