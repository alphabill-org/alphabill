package state

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
)

type (
	ShardState interface {
		Add(id types.UnitID, u *Unit) error
		Get(id types.UnitID) (*Unit, error)
		Update(id types.UnitID, unit *Unit) error
		Delete(id types.UnitID) error
	}

	Action func(s ShardState, hashAlgorithm crypto.Hash) error

	// UpdateFunction is a function for updating the data of an item. Taken in previous UnitData and returns new UnitData.
	UpdateFunction func(data types.UnitData) (newData types.UnitData, err error)
)

// AddUnit adds a new unit with given identifier, owner condition, unit data.
func AddUnit(id types.UnitID, bearer types.PredicateBytes, data types.UnitData) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		b := bytes.Clone(bearer)
		d := copyData(data)

		unitDataSummaryValue := d.SummaryValueInput()
		hasher := hashAlgorithm.New()
		hasher.Write(id)
		hasher.Write(util.Uint64ToBytes(unitDataSummaryValue))
		hasher.Write(make([]byte, hashAlgorithm.Size()))
		hasher.Write(util.Uint64ToBytes(0))
		hasher.Write(make([]byte, hashAlgorithm.Size()))
		hasher.Write(util.Uint64ToBytes(0))
		subTreeSummaryHash := hasher.Sum(nil)
		u := &Unit{
			logs:                []*Log{},
			bearer:              b,
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

// AddUnitWithLock adds a new unit with given identifier, owner condition, unit data and lock.
func AddUnitWithLock(id types.UnitID, bearer types.PredicateBytes, data types.UnitData, l []byte) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		b := bytes.Clone(bearer)
		d := copyData(data)

		unitDataSummaryValue := d.SummaryValueInput()
		hasher := hashAlgorithm.New()
		hasher.Write(id)
		hasher.Write(util.Uint64ToBytes(unitDataSummaryValue))
		hasher.Write(make([]byte, hashAlgorithm.Size()))
		hasher.Write(util.Uint64ToBytes(0))
		hasher.Write(make([]byte, hashAlgorithm.Size()))
		hasher.Write(util.Uint64ToBytes(0))
		subTreeSummaryHash := hasher.Sum(nil)
		u := &Unit{
			logs:                []*Log{},
			bearer:              b,
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

		cloned := u.Clone()
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

// SetOwner changes the owner of the item, leaves data as is
func SetOwner(id types.UnitID, bearer types.PredicateBytes) Action {
	return func(s ShardState, hashAlgorithm crypto.Hash) error {
		if id == nil {
			return errors.New("id is nil")
		}
		u, err := s.Get(id)
		if err != nil {
			return fmt.Errorf("failed to find unit: %w", err)
		}

		cloned := u.Clone()
		cloned.bearer = bearer
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

		if u.stateLockTx != nil && stateLockTx != nil {
			return errors.New("unit already has a state lock")
		}
		cloned := u.Clone()
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
