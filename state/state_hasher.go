package state

import (
	"crypto"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/tree/mt"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
)

// stateHasher calculates the root hash of the state tree (see "Invariants of the State Tree" chapter from the
// yellowpaper for more information).
type stateHasher struct {
	avl.PostOrderCommitTraverser[types.UnitID, Unit]
	hashAlgorithm crypto.Hash
}

func newStateHasher(hashAlgorithm crypto.Hash) *stateHasher {
	return &stateHasher{hashAlgorithm: hashAlgorithm}
}

// Traverse visits changed nodes in the state tree and recalculates a new root hash of the state tree.
// Executed when the State.Commit function is called.
func (p *stateHasher) Traverse(n *avl.Node[types.UnitID, Unit]) error {
	if n == nil {
		return nil
	}
	unit, err := ToUnitV1(n.Value())
	if err != nil {
		return fmt.Errorf("failed to get unit: %w", err)
	}
	if n.Clean() && unit.summaryCalculated {
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
	lv, lh, err := getSubTreeSummary(left)
	if err != nil {
		return err
	}
	rv, rh, err := getSubTreeSummary(right)
	if err != nil {
		return err
	}
	unitDataSummaryInputValue, err := getSummaryValueInput(n)
	if err != nil {
		return err
	}
	unit.subTreeSummaryValue = unitDataSummaryInputValue + lv + rv

	// h - subtree summary hash
	hasher := abhash.New(p.hashAlgorithm.New())
	hasher.Write(n.Key())
	hasher.Write(unit.logsHash)
	hasher.Write(unit.subTreeSummaryValue)
	hasher.Write(lh)
	hasher.Write(lv)
	hasher.Write(rh)
	hasher.Write(rv)

	unit.subTreeSummaryHash, err = hasher.Sum()
	if err != nil {
		return err
	}
	unit.summaryCalculated = true
	p.SetClean(n)
	return nil
}
