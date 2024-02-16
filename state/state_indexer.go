package state

import (
	"fmt"

	"github.com/alphabill-org/alphabill/types"
)

// stateIndexer traverses the state tree and constructs an index using the keyExtractor
type (
	stateIndexer[T comparable] struct {
		index        Index[T]
		err          error
		keyExtractor KeyExtractor[T]
	}

	Index[T comparable]        map[T][]types.UnitID
	KeyExtractor[T comparable] func(unit *Unit) (T, error)
)

func (s *stateIndexer[T]) Traverse(n *node) {
	if n == nil || s.err != nil {
		return
	}
	s.Traverse(n.Left())
	s.Traverse(n.Right())

	unit := n.Value()
	key, err := s.keyExtractor(unit)
	if err != nil {
		s.err = fmt.Errorf("failed to extract index key: %w", err)
		return
	}
	var zero T
	if key != zero {
		s.index[key] = append(s.index[key], n.Key())
	}
}

func CreateIndex[T comparable](s *State, ke KeyExtractor[T]) (Index[T], error) {
	indexer := &stateIndexer[T]{
		index: Index[T]{},
		keyExtractor: ke,
	}
	s.Traverse(indexer)

	if indexer.err != nil {
		return nil, fmt.Errorf("failed to traverse state tree: %w", indexer.err)
	}

	return indexer.index, nil
}

func (s *State) CreateIndex(ke KeyExtractor[string]) (Index[string], error) {
	return CreateIndex(s, ke)
}
