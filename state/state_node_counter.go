package state

type (
	stateNodeCounter struct {
		nodeCount uint64
	}
)

func NewStateNodeCounter() *stateNodeCounter {
	return &stateNodeCounter{}
}

func (s *stateNodeCounter) Traverse(n *Node) {
	if n == nil {
		return
	}
	s.Traverse(n.Left())
	s.Traverse(n.Right())
	s.nodeCount++
}

func (s *stateNodeCounter) NodeCount() uint64 {
	return s.nodeCount
}
