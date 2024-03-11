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
	// OwnerIndexer manages index of unit owners based on txsystem state.
	OwnerIndexer struct {
		log *slog.Logger

		// mu lock on ownerUnits
		mu         sync.RWMutex
		ownerUnits map[string][]types.UnitID
	}

	IndexWriter interface {
		LoadState(s txsystem.StateReader) error
		IndexBlock(b *types.Block, s StateProvider) error
	}

	IndexReader interface {
		GetOwnerUnits(ownerID []byte) ([]types.UnitID, error)
	}

	StateProvider interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
	}
)

func NewOwnerIndexer(l *slog.Logger) *OwnerIndexer {
	return &OwnerIndexer{
		log:        l,
		ownerUnits: map[string][]types.UnitID{},
	}
}

// GetOwnerUnits returns all unit ids for given owner.
func (o *OwnerIndexer) GetOwnerUnits(ownerID []byte) ([]types.UnitID, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.ownerUnits[string(ownerID)], nil
}

// LoadState fills the index from state.
func (o *OwnerIndexer) LoadState(s txsystem.StateReader) error {
	index, err := s.CreateIndex(extractOwnerID)
	if err != nil {
		return fmt.Errorf("failed to create ownerID index: %w", err)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ownerUnits = index
	return nil
}

// IndexBlock updates the index based on current committed state and transactions in a block (changed units).
func (o *OwnerIndexer) IndexBlock(b *types.Block, s StateProvider) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	for _, tx := range b.Transactions {
		for _, unitID := range tx.ServerMetadata.TargetUnits {
			unit, err := s.GetUnit(unitID, true)
			if err != nil {
				return fmt.Errorf("failed to load unit: %w", err)
			}
			unitLogs := unit.Logs()
			if len(unitLogs) == 0 {
				o.log.Error(fmt.Sprintf("cannot index unit owners, unit logs is empty, unitID=%x", unitID))
				continue
			}
			if err := o.indexUnit(unitID, unitLogs); err != nil {
				return fmt.Errorf("failed to index unit owner for unit [%s] cause: %w", unitID, err)
			}
		}
	}
	return nil
}

func (o *OwnerIndexer) indexUnit(unitID types.UnitID, logs []*state.Log) error {
	// logs - tx logs that changed the unit
	// if unit was created in this round:
	//   logs[0] - tx that created the unit
	//   logs[1..n] - txs changing the unit in current round
	// if unit existed before this round:
	//   logs[0] - last tx that changed the unit from previous rounds
	//   logs[1..n] - txs changing the unit in current round
	currOwnerPredicate := logs[len(logs)-1].NewBearer
	if err := o.addOwnerIndex(unitID, currOwnerPredicate); err != nil {
		return fmt.Errorf("failed to add owner index: %w", err)
	}
	if len(logs) > 1 {
		prevOwnerPredicate := logs[0].NewBearer
		if err := o.delOwnerIndex(unitID, prevOwnerPredicate); err != nil {
			return fmt.Errorf("failed to remove owner index: %w", err)
		}
	}
	return nil
}

func (o *OwnerIndexer) addOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID, err := extractOwnerIDFromPredicate(ownerPredicate)
	if err != nil {
		return fmt.Errorf("failed to extract owner id: %w", err)
	}
	if ownerID != "" {
		o.ownerUnits[ownerID] = append(o.ownerUnits[ownerID], unitID)
	}
	return nil
}

func (o *OwnerIndexer) delOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID, err := extractOwnerIDFromPredicate(ownerPredicate)
	if err != nil {
		return fmt.Errorf("failed to extract owner id: %w", err)
	}
	if ownerID == "" {
		return nil
	}
	unitIDs := o.ownerUnits[ownerID]
	for i, uid := range unitIDs {
		if uid.Eq(unitID) {
			unitIDs = slices.Delete(unitIDs, i, i+1)
			break
		}
	}
	if len(unitIDs) == 0 {
		// no units for owner, delete map key
		delete(o.ownerUnits, ownerID)
	} else {
		// update the removed list
		o.ownerUnits[ownerID] = unitIDs
	}
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
