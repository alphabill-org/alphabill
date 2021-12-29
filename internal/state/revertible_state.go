package state

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"github.com/holiman/uint256"
)

type (
	revertibleState struct {
		unitsTree UnitsTree
		changes   []func() error
	}

	ID             *uint256.Int
	Predicate      []byte
	Data           interface{}
	UpdateFunction func(data Data) (newData Data)

	UnitsTree interface {
		Delete(id ID) error
		Get(id ID) (owner Predicate, data Data, err error)
		Set(id ID, owner Predicate, data Data) error
		SetOwner(id ID, owner Predicate) error
		SetData(id ID, data Data) error
		Exists(id ID) (bool, error)
	}
)

// NewRevertible creates a state that can be reverted. See Revert and Commit methods for details.
func NewRevertible(unitsTree UnitsTree) *revertibleState {
	return &revertibleState{
		unitsTree: unitsTree,
		changes:   []func() error{},
	}
}

// AddItem adds new element to the state. Id must not exist in the state.
func (r *revertibleState) AddItem(id ID, owner Predicate, data Data) error {
	exists, err := r.unitsTree.Exists(id)
	if err != nil {
		return errors.Wrapf(err, "item exists check failed. ID: %d", id)
	}
	if exists {
		return errors.Errorf("cannot add item that already exists. ID: %d", id)
	}

	err = r.unitsTree.Set(id, owner, data)
	if err != nil {
		return errors.Wrapf(err, "failed to set data, ID: %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.Delete(id)
	})
	return nil
}

func (r *revertibleState) DeleteItem(id ID) error {
	owner, data, err := r.unitsTree.Get(id)
	if err != nil {
		return errors.Wrapf(err, "deleting item that does not exist. ID %d", id)
	}
	err = r.unitsTree.Delete(id)
	if err != nil {
		return errors.Wrapf(err, "deleting failed. ID %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.Set(id, owner, data)
	})
	return nil
}

func (r *revertibleState) SetOwner(id ID, owner Predicate) error {
	oldOwner, _, err := r.unitsTree.Get(id)
	if err != nil {
		return errors.Wrapf(err, "setting owner of item that does not exist. ID %d", id)
	}
	err = r.unitsTree.SetOwner(id, owner)
	if err != nil {
		return errors.Wrapf(err, "setting owner failed. ID %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.SetOwner(id, oldOwner)
	})
	return nil
}

func (r *revertibleState) UpdateData(id ID, f UpdateFunction) error {
	_, oldData, err := r.unitsTree.Get(id)
	if err != nil {
		return errors.Wrapf(err, "updating data of item that does not exist. ID %d", id)
	}
	newData := f(oldData)
	err = r.unitsTree.SetData(id, newData)
	if err != nil {
		return errors.Wrapf(err, "setting data failed. ID %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.SetData(id, oldData)
	})
	return nil
}

// Revert reverts all changes since the last commit.
// If any of the revert calls fail, it will return an error
func (r *revertibleState) Revert() error {
	for _, c := range r.changes {
		if err := c(); err != nil {
			return errors.Wrapf(err, "revert failed")
		}
	}
	r.resetChanges()
	return nil
}

// Commit commits the changes. All changes done before the Commit cannot be reverted.
func (r *revertibleState) Commit() {
	r.resetChanges()
}

func (r *revertibleState) resetChanges() {
	r.changes = []func() error{}
}
