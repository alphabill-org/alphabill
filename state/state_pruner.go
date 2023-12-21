package state

import "bytes"

type (
	statePruner struct {
		prunedTree *tree
	}
)

func newStatePruner(prunedTree *tree) *statePruner {
	return &statePruner{prunedTree: prunedTree}
}

func (s *statePruner) Traverse(n *node) {
	if n == nil {
		return
	}
	s.Traverse(n.Left())
	s.Traverse(n.Right())

	unit := n.Value()
	logSize := len(unit.logs)
	if logSize <= 1 {
		return
	}

	latestLog := unit.logs[logSize-1]
	clonedUnit := unit.Clone()
	clonedUnit.logs = []*Log{{
		TxRecordHash:       nil,
		UnitLedgerHeadHash: bytes.Clone(latestLog.UnitLedgerHeadHash),
		NewBearer:          bytes.Clone(unit.Bearer()),
		NewUnitData:        copyData(unit.Data()),
	}}
	s.prunedTree.Update(n.Key(), clonedUnit)
}
