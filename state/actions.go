package state

import (
	"crypto"
	"errors"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
)

type (
	ShardState interface {
		Add(id types.UnitID, u Unit) error
		Get(id types.UnitID) (Unit, error)
		Update(id types.UnitID, unit Unit) error
		Delete(id types.UnitID) error
	}

	Action func(s ShardState, hashAlgorithm crypto.Hash) error

	// UpdateFunction is a function for updating the data of an item. Taken in previous UnitData and returns new UnitData.
	UpdateFunction func(data types.UnitData) (newData types.UnitData, err error)
)

// AddUnit adds a new unit with given identifier, owner predicate, unit data.
func AddUnit(id types.UnitID, data types.UnitData) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		d := copyData(data)

		unitDataSummaryValue := d.SummaryValueInput()
		subTreeSummaryHash, err := summaryHashForNewUnit(hashAlgorithm, id, unitDataSummaryValue)
		if err != nil {
			return fmt.Errorf("add unit: unable to calculate subtree summary hash: %w", err)
		}
		u := &UnitV1{
			logs:                []*Log{},
			data:                d,
			subTreeSummaryValue: unitDataSummaryValue,
			subTreeSummaryHash:  subTreeSummaryHash,
		}
		if err := s.Add(id, u); err != nil {
			return fmt.Errorf("unable to add unit: %w", err)
		}
		return nil
	}
}

// AddOrPromoteUnit adds a new unit or promotes an existing dummy unit.
func AddOrPromoteUnit(id types.UnitID, data types.UnitData) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		d := copyData(data)
		summaryValue := d.SummaryValueInput()
		summaryHash, err := summaryHashForNewUnit(hashAlgorithm, id, summaryValue)
		if err != nil {
			return fmt.Errorf("add unit: unable to calculate subtree summary hash: %w", err)
		}

		u := &UnitV1{
			logs:                []*Log{},
			data:                d,
			subTreeSummaryValue: summaryValue,
			subTreeSummaryHash:  summaryHash,
		}

		existingUnit, err := s.Get(id)
		if err != nil && !errors.Is(err, avl.ErrNotFound) {
			return fmt.Errorf("add unit: unable to get existing unit: %w", err)
		}
		if existingUnit != nil {
			unitV1, err := ToUnitV1(existingUnit)
			if err != nil {
				return fmt.Errorf("unable to parse existing unit: %w", err)
			}
			if !unitV1.IsDummy() {
				return fmt.Errorf("non-dummy unit already exists: %s", id)
			}
			if err := s.Update(id, u); err != nil {
				return fmt.Errorf("unable to update dummy unit: %w", err)
			}
			return nil
		} else {
			if err := s.Add(id, u); err != nil {
				return fmt.Errorf("unable to add unit: %w", err)
			}
			return nil
		}
	}
}

func summaryHashForNewUnit(hashAlgorithm crypto.Hash, id types.UnitID, unitDataSummaryValue uint64) ([]byte, error) {
	// h = H(ι, ⊥, V; ⊥, V0; ⊥, V0)
	hasher := abhash.New(hashAlgorithm.New())
	hasher.Write(id)
	hasher.Write(nil)
	hasher.Write(unitDataSummaryValue)
	hasher.Write(nil)
	hasher.Write(uint64(0))
	hasher.Write(nil)
	hasher.Write(uint64(0))
	return hasher.Sum()
}

// AddUnitWithLock adds a new unit with given identifier, unit data and lock.
func AddUnitWithLock(id types.UnitID, data types.UnitData, l []byte) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		var unitDataSummaryValue uint64
		var clonedData types.UnitData
		if data != nil {
			clonedData = copyData(data)
			unitDataSummaryValue = clonedData.SummaryValueInput()
		}
		subTreeSummaryHash, err := summaryHashForNewUnit(hashAlgorithm, id, unitDataSummaryValue)
		if err != nil {
			return fmt.Errorf("add unit: unable to calculate subtree summary hash: %w", err)
		}
		u := &UnitV1{
			logs:                []*Log{},
			data:                clonedData,
			stateLockTx:         l,
			subTreeSummaryValue: unitDataSummaryValue,
			subTreeSummaryHash:  subTreeSummaryHash,
		}
		if err := s.Add(id, u); err != nil {
			return fmt.Errorf("unable to add unit: %w", err)
		}
		return nil
	}
}

// AddDummyUnit adds a new "dummy" unit with the given identifier.
// A dummy unit is a unit without unit data.
func AddDummyUnit(id types.UnitID) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		u := &UnitV1{
			logs: []*Log{},
		}
		if err := s.Add(id, u); err != nil {
			return fmt.Errorf("failed to add unit: %w", err)
		}
		return nil
	}
}

// UpdateUnitData changes the data of the item, leaves owner as is.
func UpdateUnitData(id types.UnitID, f UpdateFunction) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if f == nil {
			return errors.New("update function is nil")
		}
		u, err := s.Get(id)
		if err != nil {
			return fmt.Errorf("failed to get unit: %w", err)
		}

		cloned, err := ToUnitV1(u.Clone())
		if err != nil {
			return fmt.Errorf("unable to parse cloned unit: %w", err)
		}
		newData, err := f(cloned.data)
		if err != nil {
			return fmt.Errorf("unable to update unit data: %w", err)
		}
		cloned.data = newData
		if err = s.Update(id, cloned); err != nil {
			return fmt.Errorf("unable to update unit: %w", err)
		}
		return nil
	}
}

// SetStateLock sets new state lock and increments counter
func SetStateLock(id types.UnitID, stateLockTx []byte) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		if stateLockTx == nil {
			return errors.New("stateLockTx is nil")
		}
		u, err := s.Get(id)
		if err != nil {
			return fmt.Errorf("failed to find unit: %w", err)
		}
		unit, err := ToUnitV1(u)
		if err != nil {
			return fmt.Errorf("unable to parse unit: %w", err)
		}
		if unit.stateLockTx != nil {
			return errors.New("unit already has a state lock")
		}
		cloned, err := ToUnitV1(unit.Clone())
		if err != nil {
			return fmt.Errorf("unable to parse cloned unit: %w", err)
		}
		cloned.stateLockTx = stateLockTx
		if err = s.Update(id, cloned); err != nil {
			return fmt.Errorf("unable to update unit: %w", err)
		}
		return nil
	}
}

// RemoveStateLock removes the existing state lock and increments counter
func RemoveStateLock(id types.UnitID) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		u, err := s.Get(id)
		if err != nil {
			return fmt.Errorf("failed to find unit: %w", err)
		}
		unit, err := ToUnitV1(u)
		if err != nil {
			return fmt.Errorf("unable to parse unit: %w", err)
		}
		if unit.stateLockTx == nil {
			return errors.New("unit does not have a state lock")
		}
		cloned, err := ToUnitV1(unit.Clone())
		if err != nil {
			return fmt.Errorf("unable to parse cloned unit: %w", err)
		}
		cloned.stateLockTx = nil
		if err = s.Update(id, cloned); err != nil {
			return fmt.Errorf("failed to update unit: %w", err)
		}
		return nil
	}
}

// DeleteUnit removes the unit from the state with given identifier.
func DeleteUnit(id types.UnitID) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		if err := s.Delete(id); err != nil {
			return fmt.Errorf("unable to delete unit: %w", err)
		}
		return nil
	}
}

// MarkForDeletion marks the given unit to be deleted at future round i.e. logical delete.
// Logically deleted units cannot be used.
func MarkForDeletion(id types.UnitID, deletionRound uint64) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		u, err := s.Get(id)
		if err != nil {
			return fmt.Errorf("failed to find unit: %w", err)
		}
		unit, err := ToUnitV1(u)
		if err != nil {
			return fmt.Errorf("unable to parse unit: %w", err)
		}
		cloned, err := ToUnitV1(unit.Clone())
		if err != nil {
			return fmt.Errorf("unable to parse cloned unit: %w", err)
		}
		cloned.deletionRound = deletionRound
		if err = s.Update(id, cloned); err != nil {
			return fmt.Errorf("failed to update unit: %w", err)
		}
		return nil
	}
}

func copyData(data types.UnitData) types.UnitData {
	if data == nil {
		return nil
	}
	return data.Copy()
}
