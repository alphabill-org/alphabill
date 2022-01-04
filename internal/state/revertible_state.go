package state

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state/tree"
	"github.com/holiman/uint256"
)

type (
	revertibleState struct {
		unitsTree UnitsTree
		changes   []func() error
	}

	UpdateFunction func(data tree.Data) (newData tree.Data)

	UnitsTree interface {
		Delete(id *uint256.Int) error
		Get(id *uint256.Int) (owner tree.Predicate, data tree.Data, stateHash []byte, err error)
		Set(id *uint256.Int, owner tree.Predicate, data tree.Data, stateHash []byte) error
		SetOwner(id *uint256.Int, owner tree.Predicate, stateHash []byte) error
		SetData(id *uint256.Int, data tree.Data, stateHash []byte) error
		Exists(id *uint256.Int) (bool, error)
		GetRootHash() []byte
		GetSummaryValue() tree.SummaryValue
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
func (r *revertibleState) AddItem(id *uint256.Int, owner tree.Predicate, data tree.Data, stateHash []byte) error {
	exists, err := r.unitsTree.Exists(id)
	if err != nil {
		return errors.Wrapf(err, "item exists check failed. ID: %d", id)
	}
	if exists {
		return errors.Errorf("cannot add item that already exists. ID: %d", id)
	}

	err = r.unitsTree.Set(id, owner, data, stateHash)
	if err != nil {
		return errors.Wrapf(err, "failed to set data, ID: %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.Delete(id)
	})
	return nil
}

func (r *revertibleState) DeleteItem(id *uint256.Int) error {
	owner, data, stateHash, err := r.unitsTree.Get(id)
	if err != nil {
		return errors.Wrapf(err, "deleting item that does not exist. ID %d", id)
	}
	err = r.unitsTree.Delete(id)
	if err != nil {
		return errors.Wrapf(err, "deleting failed. ID %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.Set(id, owner, data, stateHash)
	})
	return nil
}

func (r *revertibleState) SetOwner(id *uint256.Int, owner tree.Predicate, stateHash []byte) error {
	oldOwner, _, oldStateHash, err := r.unitsTree.Get(id)
	if err != nil {
		return errors.Wrapf(err, "setting owner of item that does not exist. ID %d", id)
	}
	err = r.unitsTree.SetOwner(id, owner, stateHash)
	if err != nil {
		return errors.Wrapf(err, "setting owner failed. ID %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.SetOwner(id, oldOwner, oldStateHash)
	})
	return nil
}

func (r *revertibleState) UpdateData(id *uint256.Int, f UpdateFunction, stateHash []byte) error {
	_, oldData, oldStateHash, err := r.unitsTree.Get(id)
	if err != nil {
		return errors.Wrapf(err, "updating data of item that does not exist. ID %d", id)
	}
	newData := f(oldData)
	err = r.unitsTree.SetData(id, newData, stateHash)
	if err != nil {
		return errors.Wrapf(err, "setting data failed. ID %d", id)
	}
	r.changes = append(r.changes, func() error {
		return r.unitsTree.SetData(id, oldData, oldStateHash)
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
