package partition

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
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
	ownerID, err := extractOwnerIDFromPredicate(ownerPredicate)
	if err != nil {
		return fmt.Errorf("failed to extract owner id: %w", err)
	}
	if ownerID != "" {
		p.ownerUnits[ownerID] = append(p.ownerUnits[ownerID], unitID)
	}
	return nil
}

func (p *OwnerIndexer) delOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID, err := extractOwnerIDFromPredicate(ownerPredicate)
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
func (p *OwnerIndexer) LoadState(s txsystem.StateReader) error {
	index, err := s.CreateIndex(extractOwnerID)
	if err != nil {
		return fmt.Errorf("failed to create ownerID index: %w", err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ownerUnits = index
	return nil
}

func extractOwnerID(unit *state.Unit) (string, error) {
	return extractOwnerIDFromPredicate(unit.Bearer())
}

func extractOwnerIDFromPredicate(predicateBytes []byte) (string, error) {
	predicate, err := predicates.ExtractPredicate(predicateBytes)
	if err != nil {
		return "", fmt.Errorf("failed to extract predicate '%X': %w", predicateBytes, err)
	}

	if !templates.IsP2pkhTemplate(predicate) {
		// do not index non-p2pkh predicates
		return "", nil
	}
	// for p2pkh predicates use pubkey hash as the owner id
	return string(predicate.Params), nil
}
