package state

import (
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	InorderTraverser struct {
		visit func(unitID types.UnitID, unit Unit) error
	}
)

func NewInorderTraverser(visitFn func(unitID types.UnitID, unit Unit) error) *InorderTraverser {
	return &InorderTraverser{visit: visitFn}
}

func (s *InorderTraverser) Traverse(n *node) error {
	if n == nil {
		return nil
	}
	if err := s.Traverse(n.Left()); err != nil {
		return err
	}
	if err := s.visit(n.Key(), n.Value()); err != nil {
		return err
	}
	if err := s.Traverse(n.Right()); err != nil {
		return err
	}
	return nil
}
