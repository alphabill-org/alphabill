package state

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/tree/mt"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/tree/avl"
)

// stateHasher calculates the root hash of the state tree (see "Invariants of the State Tree" chapter from the
// yellowpaper for more information).
type stateHasher struct {
	avl.PostOrderCommitTraverser[types.UnitID, *Unit]
	hashAlgorithm crypto.Hash
}

func newStateHasher(hashAlgorithm crypto.Hash) *stateHasher {
	return &stateHasher{hashAlgorithm: hashAlgorithm}
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
	// Skip this step if state has been recovered from file and logsHash is already present.
	if unit.logsHash == nil {
		merkleTree := mt.New(p.hashAlgorithm, unit.logs)
		unit.logsHash = merkleTree.GetRootHash()
	}

	// owner update
	unit.owner = unit.latestUnitBearer()

	unit.stateLockTx = unit.latestStateLockTx()

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
	hasher.Write(unit.logsHash)
	hasher.Write(util.Uint64ToBytes(unit.subTreeSummaryValue))
	hasher.Write(getSubTreeSummaryHash(left))
	hasher.Write(util.Uint64ToBytes(leftSummary))
	hasher.Write(getSubTreeSummaryHash(right))
	hasher.Write(util.Uint64ToBytes(rightSummary))

	unit.subTreeSummaryHash = hasher.Sum(nil)
	unit.summaryCalculated = true
	p.SetClean(n)
}
