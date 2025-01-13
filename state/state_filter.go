package state

import (
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	Filter struct {
		filterFn        func(unitID types.UnitID, unit VersionedUnit) (bool, error)
		filteredUnitIDs []types.UnitID
	}
)

func NewFilter(filterFn func(unitID types.UnitID, unit VersionedUnit) (bool, error)) *Filter {
	return &Filter{filterFn: filterFn}
}

func (s *Filter) Traverse(n *node) error {
	if n == nil {
		return nil
	}
	if err := s.Traverse(n.Left()); err != nil {
		return err
	}
	// inorder traversal so that result is in sorted order
	ok, err := s.filterFn(n.Key(), n.Value())
	if err != nil {
		return err
	}
	if ok {
		s.filteredUnitIDs = append(s.filteredUnitIDs, n.Key())
	}
	if err := s.Traverse(n.Right()); err != nil {
		return err
	}
	return nil
}

func (s *Filter) FilteredUnitIDs() []types.UnitID {
	return s.filteredUnitIDs
}
