package state

import (
	"crypto"

	"github.com/alphabill-org/alphabill/validator/internal/mt"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/alphabill-org/alphabill/validator/internal/util"
	"github.com/alphabill-org/alphabill/validator/pkg/tree/avl"
)

// stateHasher calculates the root hash of the state tree (see "Invariants of the State Tree" chapter from the
// yellowpaper for more information).
type stateHasher struct {
	avl.PostOrderCommitTraverser[types.UnitID, *Unit]
	hashAlgorithm crypto.Hash
}

// Traverse visits changed nodes in the state tree and recalculates a new root hash of the state tree.
// Executed when the State.Commit function is called.
func (p *stateHasher) Traverse(n *avl.Node[types.UnitID, *Unit]) {
	if n == nil || (n.Clean() && n.Value().summaryCalculated) {
		return
	}
	var left = n.Left()
	var right = n.Right()
	p.Traverse(left)
	p.Traverse(right)

	unit := n.Value()

	// h_s - calculate state log root hash
	merkleTree := mt.New(p.hashAlgorithm, unit.logs)
	unit.logRoot = merkleTree.GetRootHash()

	// bearer update
	unit.bearer = unit.latestUnitBearer()

	// D - unit data
	unit.data = unit.latestUnitData()

	// V - calculate summary value
	leftSummary := getSubTreeSummaryValue(left)
	rightSummary := getSubTreeSummaryValue(right)
	unitDataSummaryInputValue := getSummaryValueInput(n)
	unit.subTreeSummaryValue = unitDataSummaryInputValue + leftSummary + rightSummary

	// h - subtree summary hash
	hasher := p.hashAlgorithm.New()
	hasher.Write(n.Key())
	hasher.Write(unit.logRoot)
	hasher.Write(util.Uint64ToBytes(unit.subTreeSummaryValue))
	hasher.Write(getSubTreeSummaryHash(left))
	hasher.Write(util.Uint64ToBytes(leftSummary))
	hasher.Write(getSubTreeSummaryHash(right))
	hasher.Write(util.Uint64ToBytes(rightSummary))

	unit.subTreeSummaryHash = hasher.Sum(nil)
	p.SetClean(n)
	n.Value().summaryCalculated = true
}
