package state

import (
	"errors"
	"testing"

	"github.com/holiman/uint256"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

var (
	id                        = uint256.NewInt(1)
	owner      Predicate      = []byte("owner")
	oldOwner   Predicate      = []byte("old owner")
	data                      = "data"
	oldData                   = "old data"
	newData                   = "new data"
	updateFunc UpdateFunction = func(data Data) Data {
		return newData
	}
)

func TestRevertible_Empty(t *testing.T) {
	utree := new(MockUnitsTree)
	rstate := NewRevertible(utree)
	require.NotNil(t, rstate)
	mock.AssertExpectationsForObjects(t, utree)
}

func TestRevertible_AddItem(t *testing.T) {
	testData := []struct {
		name            string
		expectForAdd    func(utree *MockUnitsTree)
		addErr          error
		expectForRevert func(utree *MockUnitsTree)
		revertErr       error
	}{
		{
			name: "happy flow",
			expectForAdd: func(utree *MockUnitsTree) {
				utree.On("Exists", id).Return(false, nil)
				utree.On("Set", id, owner, data).Return(nil)
			},
			addErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("Delete", id).Return(nil)
			},
			revertErr: nil,
		},
		{
			name: "item exists",
			expectForAdd: func(utree *MockUnitsTree) {
				utree.On("Exists", id).Return(true, nil)
			},
			addErr:          errors.New("cannot add item that already exists"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "set item fails",
			expectForAdd: func(utree *MockUnitsTree) {
				utree.On("Exists", id).Return(false, nil)
				utree.On("Set", id, owner, data).Return(errors.New("set item failed"))
			},
			addErr:          errors.New("set item failed"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "revert: delete fails",
			expectForAdd: func(utree *MockUnitsTree) {
				utree.On("Exists", id).Return(false, nil)
				utree.On("Set", id, owner, data).Return(nil)
			},
			addErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("Delete", id).Return(errors.New("delete fails"))
			},
			revertErr: errors.New("delete fails"),
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			utree := new(MockUnitsTree)
			rstate := NewRevertible(utree)

			// Add item
			tt.expectForAdd(utree)
			err := rstate.AddItem(id, owner, data)
			requireErrorMatches(t, tt.addErr, err)
			mock.AssertExpectationsForObjects(t, utree)

			// Reset mock for revert, so the log is cleaner.
			resetTreeMock(utree)
			tt.expectForRevert(utree)
			err = rstate.Revert()
			requireErrorMatches(t, tt.revertErr, err)
			mock.AssertExpectationsForObjects(t, utree)
		})
	}
}

func TestRevertible_DeleteItem(t *testing.T) {
	testData := []struct {
		name            string
		expectForDelete func(utree *MockUnitsTree)
		deleteErr       error
		expectForRevert func(utree *MockUnitsTree)
		revertErr       error
	}{
		{
			name: "happy flow",
			expectForDelete: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(owner, data, nil)
				utree.On("Delete", id).Return(nil)
			},
			deleteErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("Set", id, owner, data).Return(nil)
			},
			revertErr: nil,
		},
		{
			name: "get fails",
			expectForDelete: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(nil, nil, errors.New("get failed"))
			},
			deleteErr:       errors.New("get failed"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "delete fails",
			expectForDelete: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(owner, data, nil)
				utree.On("Delete", id).Return(errors.New("delete failed"))
			},
			deleteErr:       errors.New("delete failed"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "revert: set fails",
			expectForDelete: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(owner, data, nil)
				utree.On("Delete", id).Return(nil)
			},
			deleteErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("Set", id, owner, data).Return(errors.New("set failed"))
			},
			revertErr: errors.New("set failed"),
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			utree := new(MockUnitsTree)
			rstate := NewRevertible(utree)

			// Delete item
			tt.expectForDelete(utree)
			err := rstate.DeleteItem(id)
			requireErrorMatches(t, tt.deleteErr, err)
			mock.AssertExpectationsForObjects(t, utree)

			// Reset mock for revert, so the log is cleaner.
			resetTreeMock(utree)
			tt.expectForRevert(utree)
			err = rstate.Revert()
			requireErrorMatches(t, tt.revertErr, err)
			mock.AssertExpectationsForObjects(t, utree)
		})
	}
}

func TestRevertible_SetOwner(t *testing.T) {
	testData := []struct {
		name              string
		expectForSetOwner func(utree *MockUnitsTree)
		setOwnerErr       error
		expectForRevert   func(utree *MockUnitsTree)
		revertErr         error
	}{
		{
			name: "happy flow",
			expectForSetOwner: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(oldOwner, data, nil)
				utree.On("SetOwner", id, owner).Return(nil)
			},
			setOwnerErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("SetOwner", id, oldOwner).Return(nil)
			},
			revertErr: nil,
		},
		{
			name: "get fails",
			expectForSetOwner: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(nil, nil, errors.New("get failed"))
			},
			setOwnerErr:     errors.New("get failed"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "set owner fails",
			expectForSetOwner: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(oldOwner, data, nil)
				utree.On("SetOwner", id, owner).Return(errors.New("set owner failed"))
			},
			setOwnerErr:     errors.New("set owner failed"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "revert: setOwner fails",
			expectForSetOwner: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(oldOwner, data, nil)
				utree.On("SetOwner", id, owner).Return(nil)
			},
			setOwnerErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("SetOwner", id, oldOwner).Return(errors.New("revert set owner failed"))
			},
			revertErr: errors.New("revert set owner failed"),
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			utree := new(MockUnitsTree)
			rstate := NewRevertible(utree)

			// Delete item
			tt.expectForSetOwner(utree)
			err := rstate.SetOwner(id, owner)
			requireErrorMatches(t, tt.setOwnerErr, err)
			mock.AssertExpectationsForObjects(t, utree)

			// Reset mock for revert, so the log is cleaner.
			resetTreeMock(utree)
			tt.expectForRevert(utree)
			err = rstate.Revert()
			requireErrorMatches(t, tt.revertErr, err)
			mock.AssertExpectationsForObjects(t, utree)
		})
	}
}

func TestRevertible_UpdateData(t *testing.T) {
	testData := []struct {
		name                string
		expectForUpdateData func(utree *MockUnitsTree)
		updateDataErr       error
		expectForRevert     func(utree *MockUnitsTree)
		revertErr           error
	}{
		{
			name: "happy flow",
			expectForUpdateData: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(owner, oldData, nil)
				utree.On("SetData", id, newData).Return(nil)
			},
			updateDataErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("SetData", id, oldData).Return(nil)
			},
			revertErr: nil,
		},
		{
			name: "get fails",
			expectForUpdateData: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(nil, nil, errors.New("get failed"))
			},
			updateDataErr:   errors.New("get failed"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "set data fails",
			expectForUpdateData: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(oldOwner, data, nil)
				utree.On("SetData", id, newData).Return(errors.New("set data failed"))
			},
			updateDataErr:   errors.New("set data failed"),
			expectForRevert: func(utree *MockUnitsTree) {},
			revertErr:       nil,
		},
		{
			name: "revert: setData fails",
			expectForUpdateData: func(utree *MockUnitsTree) {
				utree.On("Get", id).Return(oldOwner, oldData, nil)
				utree.On("SetData", id, newData).Return(nil)
			},
			updateDataErr: nil,
			expectForRevert: func(utree *MockUnitsTree) {
				utree.On("SetData", id, oldData).Return(errors.New("revert set data failed"))
			},
			revertErr: errors.New("revert set data failed"),
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			utree := new(MockUnitsTree)
			rstate := NewRevertible(utree)

			// Delete item
			tt.expectForUpdateData(utree)
			err := rstate.UpdateData(id, updateFunc)
			requireErrorMatches(t, tt.updateDataErr, err)
			mock.AssertExpectationsForObjects(t, utree)

			// Reset mock for revert, so the log is cleaner.
			resetTreeMock(utree)
			tt.expectForRevert(utree)
			err = rstate.Revert()
			requireErrorMatches(t, tt.revertErr, err)
			mock.AssertExpectationsForObjects(t, utree)
		})
	}
}

func TestRevertible_Commit(t *testing.T) {
	utree := new(MockUnitsTree)
	rstate := NewRevertible(utree)

	count := 10

	utree.On("Exists", id).Times(count).Return(false, nil)
	utree.On("Set", id, owner, data).Times(count).Return(nil)
	for i := 0; i < count; i++ {
		err := rstate.AddItem(id, owner, data)
		require.NoError(t, err)
	}
	rstate.Commit()
	mock.AssertExpectationsForObjects(t, utree)

	// Reset mock after the commit
	resetTreeMock(utree)
	err := rstate.Revert()
	require.NoError(t, err)
	// Nothing should be done with the tree
	mock.AssertExpectationsForObjects(t, utree)

	// Add more items after the commit
	utree.On("Exists", id).Times(count).Return(false, nil)
	utree.On("Set", id, owner, data).Times(count).Return(nil)
	for i := 0; i < count; i++ {
		err := rstate.AddItem(id, owner, data)
		require.NoError(t, err)
	}
	// Make sure the items get added to the tree
	mock.AssertExpectationsForObjects(t, utree)
	resetTreeMock(utree)

	// Make sure only the newly added items are reverted.
	utree.On("Delete", id).Times(count).Return(nil)
	err = rstate.Revert()
	require.NoError(t, err)
	// Now the last additions should be added.
	mock.AssertExpectationsForObjects(t, utree)
}

func requireErrorMatches(t *testing.T, expectedErr, actualErr error) {
	if expectedErr != nil {
		require.Errorf(t, expectedErr, actualErr.Error())
	} else {
		require.NoError(t, actualErr)
	}
}

func resetTreeMock(treeMock *MockUnitsTree) {
	treeMock.ExpectedCalls = []*mock.Call{}
	treeMock.Calls = []mock.Call{}
}
