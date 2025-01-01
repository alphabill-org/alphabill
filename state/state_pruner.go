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

func (s *statePruner) Traverse(n *node) error {
	if n == nil {
		return nil
	}
	if err := s.Traverse(n.Left()); err != nil {
		return err
	}
	if err := s.Traverse(n.Right()); err != nil {
		return err
	}

	unit := n.Value().GetV1()
	logSize := len(unit.logs)
	if logSize <= 1 {
		return nil
	}

	latestLog := unit.logs[logSize-1]
	clonedUnit := unit.Clone().GetV1()
	clonedUnit.logs = []*Log{{
		TxRecordHash:       nil,
		UnitLedgerHeadHash: bytes.Clone(latestLog.UnitLedgerHeadHash),
		NewUnitData:        copyData(unit.Data()),
	}}
	return s.prunedTree.Update(n.Key(), clonedUnit)
}
