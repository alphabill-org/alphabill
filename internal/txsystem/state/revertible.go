package state

import (
	"crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"github.com/holiman/uint256"
)

// TODO will be implemented, after a revertible tree is ready. Currently a placeholder.
type revertibleUnitTree struct {
	*unitTree
}

// NewRevertible creates a state tree that keeps track of changes made and allows these to be reverted with Revert method.
// When running Commit method, the changes are fixed and cannot be reverted any more.
func NewRevertible(hashAlgorithm crypto.Hash) (*revertibleUnitTree, error) {
	ut, err := New(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return &revertibleUnitTree{
		unitTree: ut,
	}, nil
}

// Revert reverts all changes since the last commit.
// If any of the revert calls fail, it will return an error
func (r *revertibleUnitTree) Revert() error {
	return errors.ErrNotImplemented
}

// Commit commits the changes, making these not revertible.
// Changes done before the Commit cannot be reverted anymore.
// Changes done after the last Commit can be reverted by Revert method.
func (r *revertibleUnitTree) Commit() {

}

// Below are overloaded method of the units Tree

// AddItem is same as normal state tree AddItem, but on revert will delete the item added.
func (r *revertibleUnitTree) AddItem(id *uint256.Int, owner Predicate, data UnitData, stateHash []byte) error {
	return errors.ErrNotImplemented
}

// DeleteItem is same as normal state tree DeleteItem, but on revert will add the item deleted.
func (r *revertibleUnitTree) DeleteItem(id *uint256.Int) error {
	return errors.ErrNotImplemented
}

// SetOwner is same as normal state tree SetOwner, but on revert will set the previous owner back.
func (r *revertibleUnitTree) SetOwner(id *uint256.Int, owner Predicate, stateHash []byte) error {
	return errors.ErrNotImplemented
}

// UpdateData is same as normal state tree UpdateData, but on revert will set the previous data back.
func (r *revertibleUnitTree) UpdateData(id *uint256.Int, f UpdateFunction, stateHash []byte) error {
	return errors.ErrNotImplemented
}
