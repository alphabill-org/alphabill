package state

import (
	"bytes"
	"fmt"
)

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

	unit, err := ToUnitV1(n.Value())
	if err != nil {
		return fmt.Errorf("failed to get unit: %w", err)
	}
	logSize := len(unit.logs)
	if logSize <= 1 {
		return nil
	}

	latestLog := unit.logs[logSize-1]
	clonedUnit, err := ToUnitV1(unit.Clone())
	if err != nil {
		return fmt.Errorf("unable to parse cloned unit: %w", err)
	}
	clonedUnit.logs = []*Log{{
		TxRecordHash:       nil,
		UnitLedgerHeadHash: bytes.Clone(latestLog.UnitLedgerHeadHash),
		NewUnitData:        copyData(unit.Data()),
	}}
	return s.prunedTree.Update(n.Key(), clonedUnit)
}
