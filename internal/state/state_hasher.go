package state

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/tree/avl"
)

// stateHasher calculates the root hash of the state tree (see "Invariants of the State Tree" chapter from the
// yellowpaper for more information).
type stateHasher struct {
	avl.PostOrderCommitTraverser[types.UnitID, *Unit]
	hashAlgorithm crypto.Hash
}

// Traverse visits changed nodes in the state tree and recalculates a new root hash of the state tree.
// Executed when the State.Commit function is called.
func (p *stateHasher) Traverse(n *avl.Node[types.UnitID, *Unit]) error {
	if n == nil || n.Clean() {
		return nil
	}
	var leftSummary uint64
	var rightSummary uint64

	var left = n.Left()
	var right = n.Right()

	if left != nil {
		if err := p.Traverse(left); err != nil {
			return err
		}
		leftSummary = left.Value().subTreeSummaryValue
	}
	if right != nil {
		if err := p.Traverse(right); err != nil {
			return err
		}
		rightSummary = right.Value().subTreeSummaryValue
	}

	unit := n.Value()

	// h_s - calculate state log root hash
	merkleTree, err := mt.New(p.hashAlgorithm, unit.logs)
	if err != nil {
		return fmt.Errorf("unable to calculate state log hash for unit %v: %w", n.Key(), err)
	}
	unit.logRoot = merkleTree.GetRootHash()

	// bearer update
	unit.bearer = unit.latestUnitBearer()

	// D - unit data
	unit.data = unit.latestUnitData()

	// V - calculate summary value
	unitDataSummaryInputValue := unit.data.SummaryValueInput()
	unit.subTreeSummaryValue = unitDataSummaryInputValue + leftSummary + rightSummary

	// h - subtree summary hash
	hasher := p.hashAlgorithm.New()
	hasher.Write(n.Key())
	hasher.Write(unit.logRoot)
	hasher.Write(util.Uint64ToBytes(unitDataSummaryInputValue))

	if left != nil {
		hasher.Write(left.Value().subTreeSummaryHash)
		hasher.Write(util.Uint64ToBytes(leftSummary))
	} else {
		hasher.Write(make([]byte, p.hashAlgorithm.Size()))
		hasher.Write(util.Uint64ToBytes(0))
	}
	if right != nil {
		hasher.Write(right.Value().subTreeSummaryHash)
		hasher.Write(util.Uint64ToBytes(rightSummary))
	} else {
		hasher.Write(make([]byte, p.hashAlgorithm.Size()))
		hasher.Write(util.Uint64ToBytes(0))
	}

	unit.subTreeSummaryHash = hasher.Sum(nil)
	p.SetClean(n)
	return nil
}
