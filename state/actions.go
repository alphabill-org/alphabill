package state

import (
	"crypto"
	"errors"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	ShardState interface {
		Add(id types.UnitID, u VersionedUnit) error
		Get(id types.UnitID) (VersionedUnit, error)
		Update(id types.UnitID, unit VersionedUnit) error
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
		hasher := abhash.New(hashAlgorithm.New())
		// h = H(ι, ⊥, V; ⊥, V0; ⊥, V0)
		hasher.Write(id)
		hasher.Write(nil)
		hasher.Write(unitDataSummaryValue)
		hasher.Write(nil)
		hasher.Write(uint64(0))
		hasher.Write(nil)
		hasher.Write(uint64(0))
		subTreeSummaryHash, err := hasher.Sum()
		if err != nil {
			return fmt.Errorf("add unit: unable to calculate subtree summary hash: %w", err)
		}
		u := &Unit{
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

// AddUnitWithLock adds a new unit with given identifier, owner predicate, unit data and lock.
func AddUnitWithLock(id types.UnitID, data types.UnitData, l []byte) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		d := copyData(data)

		unitDataSummaryValue := d.SummaryValueInput()
		hasher := abhash.New(hashAlgorithm.New())
		// h = H(ι, ⊥, V; ⊥, V0; ⊥, V0)
		hasher.Write(id)
		hasher.Write(nil)
		hasher.Write(unitDataSummaryValue)
		hasher.Write(nil)
		hasher.Write(uint64(0))
		hasher.Write(nil)
		hasher.Write(uint64(0))
		subTreeSummaryHash, err := hasher.Sum()
		if err != nil {
			return fmt.Errorf("add unit: unable to calculate subtree summary hash: %w", err)
		}
		u := &Unit{
			logs:                []*Log{},
			data:                d,
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

		cloned, err := UnitV1(u.Clone())
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

// SetStateLock sets new state lock or removes the existing one and increments counter
func SetStateLock(id types.UnitID, stateLockTx []byte) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		u, err := s.Get(id)
		if err != nil {
			return fmt.Errorf("failed to find unit: %w", err)
		}

		unit, err := UnitV1(u)
		if err != nil {
			return fmt.Errorf("unable to parse unit: %w", err)
		}
		if unit.stateLockTx != nil && stateLockTx != nil {
			return errors.New("unit already has a state lock")
		}
		cloned, err := UnitV1(unit.Clone())
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

func copyData(data types.UnitData) types.UnitData {
	if data == nil {
		return nil
	}
	return data.Copy()
}
