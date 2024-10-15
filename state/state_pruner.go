package state

import "bytes"

type (
	statePruner struct {
		prunedTree *tree
		err        error
	}
)

func newStatePruner(prunedTree *tree) *statePruner {
	return &statePruner{prunedTree: prunedTree}
}

func (s *statePruner) Traverse(n *node) {
	if n == nil || s.err != nil {
		return
	}
	s.Traverse(n.Left())
	s.Traverse(n.Right())

	if s.err != nil {
		return
	}

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
		NewUnitData:        copyData(unit.Data()),
	}}
	s.err = s.prunedTree.Update(n.Key(), clonedUnit)
}

func (s *statePruner) Err() error {
	return s.err
}
