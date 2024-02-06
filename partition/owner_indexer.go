package partition

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/types"
)

type (
	OwnerIndexer struct {
		log *slog.Logger

		// mu lock on ownerUnits
		mu         sync.RWMutex
		ownerUnits map[string][]types.UnitID
	}
)

func NewOwnerIndexer(l *slog.Logger) *OwnerIndexer {
	return &OwnerIndexer{
		log:        l,
		ownerUnits: map[string][]types.UnitID{},
	}
}

// GetOwnerUnits returns all unit ids for given owner.
func (p *OwnerIndexer) GetOwnerUnits(ownerID []byte) ([]types.UnitID, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.ownerUnits[string(ownerID)], nil
}

// IndexOwner adds or updates the index entry for the given unit, and removes existing entry based on the unit logs.
func (p *OwnerIndexer) IndexOwner(unitID types.UnitID, logs []*state.Log) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(logs) == 0 {
		p.log.Error(fmt.Sprintf("cannot index unit owners, unit logs is empty, unitID=%x", unitID))
		return nil
	}
	// logs - tx logs that changed the unit
	// if unit was created in this round:
	//   logs[0] - tx that created the unit
	//   logs[1..n] - txs changing the unit in current round
	// if unit existed before this round:
	//   logs[0] - last tx that changed the unit from previous rounds
	//   logs[1..n] - txs changing the unit in current round
	currOwnerPredicate := logs[len(logs)-1].NewBearer
	if err := p.addOwnerIndex(unitID, currOwnerPredicate); err != nil {
		return fmt.Errorf("failed to add owner index: %w", err)
	}
	if len(logs) > 1 {
		prevOwnerPredicate := logs[0].NewBearer
		if err := p.delOwnerIndex(unitID, prevOwnerPredicate); err != nil {
			return fmt.Errorf("failed to remove owner index: %w", err)
		}
	}
	return nil
}

func (p *OwnerIndexer) addOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID, err := extractOwnerID(ownerPredicate)
	if err != nil {
		return fmt.Errorf("failed to extract owner id: %w", err)
	}
	if ownerID != "" {
		p.ownerUnits[ownerID] = append(p.ownerUnits[ownerID], unitID)
	}
	return nil
}

func (p *OwnerIndexer) delOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID, err := extractOwnerID(ownerPredicate)
	if err != nil {
		return fmt.Errorf("failed to extract owner id: %w", err)
	}
	if ownerID == "" {
		return nil
	}
	unitIDs := p.ownerUnits[ownerID]
	for i, uid := range unitIDs {
		if uid.Eq(unitID) {
			unitIDs = slices.Delete(unitIDs, i, i+1)
			break
		}
	}
	if len(unitIDs) == 0 {
		// no units for owner, delete map key
		delete(p.ownerUnits, ownerID)
	} else {
		// update the removed list
		p.ownerUnits[ownerID] = unitIDs
	}
	return nil
}

// LoadState fills the index from state.
func (p *OwnerIndexer) LoadState(s *state.State) error {
	t := &ownerTraverser{ownerUnits: map[string][]types.UnitID{}}
	s.Traverse(t)
	if t.err != nil {
		return fmt.Errorf("failed to traverse state tree: %w", t.err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ownerUnits = t.ownerUnits
	return nil
}

// ownerTraverser traverses state tree and records all nodes into ownerUnits map
type ownerTraverser struct {
	ownerUnits map[string][]types.UnitID
	err        error
}

func (s *ownerTraverser) Traverse(n *avl.Node[types.UnitID, *state.Unit]) {
	if n == nil || s.err != nil {
		return
	}
	s.Traverse(n.Left())
	s.Traverse(n.Right())

	unit := n.Value()
	ownerID, err := extractOwnerID(unit.Bearer())
	if err != nil {
		s.err = fmt.Errorf("failed to extract owner id: %w", err)
		return
	}
	if ownerID != "" {
		s.ownerUnits[ownerID] = append(s.ownerUnits[ownerID], n.Key())
	}
}

func extractOwnerID(ownerPredicate []byte) (string, error) {
	predicate, err := predicates.ExtractPredicate(ownerPredicate)
	if err != nil {
		return "", fmt.Errorf("failed to extract predicate '%X': %w", ownerPredicate, err)
	}
	if !templates.IsP2pkhTemplate(predicate) {
		// do not index non-p2pkh predicates
		return "", nil
	}
	// for p2pkh predicates use pubkey hash as the owner id
	return string(predicate.Params), nil
}
