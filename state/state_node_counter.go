package state

type (
	stateNodeCounter struct {
		nodeCount uint64
	}
)

func NewStateNodeCounter() *stateNodeCounter {
	return &stateNodeCounter{}
}

func (s *stateNodeCounter) Traverse(n *node) error {
	if n == nil {
		return nil
	}
	if err := s.Traverse(n.Left()); err != nil {
		return err
	}
	if err := s.Traverse(n.Right()); err != nil {
		return err
	}

	s.nodeCount++
	return nil
}

func (s *stateNodeCounter) NodeCount() uint64 {
	return s.nodeCount
}
