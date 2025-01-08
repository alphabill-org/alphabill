package partition

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
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
		GetUnit(id types.UnitID, committed bool) (state.VersionedUnit, error)
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
	index, err := s.CreateIndex(o.extractOwnerID)
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
		for _, unitID := range tx.TargetUnits() {
			unit, err := s.GetUnit(unitID, true)
			if err != nil {
				return fmt.Errorf("failed to load unit: %w", err)
			}
			unitLogs := unit.GetV1().Logs()
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
	currOwnerPredicate := logs[len(logs)-1].NewUnitData.Owner()
	if err := o.addOwnerIndex(unitID, currOwnerPredicate); err != nil {
		return fmt.Errorf("failed to add owner index: %w", err)
	}
	if len(logs) > 1 {
		prevOwnerPredicate := logs[0].NewUnitData.Owner()
		if err := o.delOwnerIndex(unitID, prevOwnerPredicate); err != nil {
			return fmt.Errorf("failed to remove owner index: %w", err)
		}
	}
	return nil
}

func (o *OwnerIndexer) addOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID := o.extractOwnerIDFromPredicate(ownerPredicate)
	if ownerID != "" {
		o.ownerUnits[ownerID] = append(o.ownerUnits[ownerID], unitID)
	}
	return nil
}

func (o *OwnerIndexer) delOwnerIndex(unitID types.UnitID, ownerPredicate []byte) error {
	ownerID := o.extractOwnerIDFromPredicate(ownerPredicate)
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

func (o *OwnerIndexer) extractOwnerID(unit state.VersionedUnit) (string, error) {
	return o.extractOwnerIDFromPredicate(unit.Data().Owner()), nil
}

func (o *OwnerIndexer) extractOwnerIDFromPredicate(predicateBytes []byte) string {
	predicate, err := predicates.ExtractPredicate(predicateBytes)
	if err != nil {
		// unit owner predicate can be arbitrary data and does not have to conform to predicate template
		o.log.Debug(fmt.Sprintf("failed to extract predicate '%X': %v", predicateBytes, err))
		return ""
	}

	if err := templates.VerifyP2pkhPredicate(predicate); err != nil {
		// do not index non-p2pkh predicates
		return ""
	}
	// for p2pkh predicates use pubkey hash as the owner id
	return string(predicate.Params)
}
