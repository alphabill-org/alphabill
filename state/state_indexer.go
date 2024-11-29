package state

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

// stateIndexer traverses the state tree and constructs an index using the keyExtractor
type (
	stateIndexer[T comparable] struct {
		index        Index[T]
		keyExtractor KeyExtractor[T]
	}

	Index[T comparable]        map[T][]types.UnitID
	KeyExtractor[T comparable] func(unit *Unit) (T, error)
)

func (s *stateIndexer[T]) Traverse(n *node) error {
	if n == nil {
		return nil
	}
	if err := s.Traverse(n.Left()); err != nil {
		return err
	}
	if err := s.Traverse(n.Right()); err != nil {
		return err
	}

	unit := n.Value()
	key, err := s.keyExtractor(unit)
	if err != nil {
		return fmt.Errorf("failed to extract index key: %w", err)
	}
	var zero T
	if key != zero {
		s.index[key] = append(s.index[key], n.Key())
	}
	return nil
}

func CreateIndex[T comparable](s *State, ke KeyExtractor[T]) (Index[T], error) {
	indexer := &stateIndexer[T]{
		index:        Index[T]{},
		keyExtractor: ke,
	}

	if err := s.Traverse(indexer); err != nil {
		return nil, fmt.Errorf("failed to traverse state tree: %w", err)
	}

	return indexer.index, nil
}

func (s *State) CreateIndex(ke KeyExtractor[string]) (Index[string], error) {
	return CreateIndex(s, ke)
}
