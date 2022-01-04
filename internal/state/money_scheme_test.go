package state

import (
	"crypto"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state/tree"

	"github.com/stretchr/testify/mock"

	"github.com/holiman/uint256"

	"github.com/stretchr/testify/require"
)

const (
	addItemId                = 0
	addItemOwner             = 1
	addItemData              = 2
	UpdateDataUpdateFunction = 1
)

func TestNewMoneyScheme(t *testing.T) {
	mockTree := new(MockUnitsTree)

	initialBill := &InitialBill{ID: uint256.NewInt(2), Value: 100, Owner: nil}
	dcMoneyAmount := uint64(222)

	// Initial bill gets set
	mockTree.On("Set", initialBill.ID, initialBill.Owner,
		&BillData{V: initialBill.Value, T: 0, Backlink: nil},
		[]byte(nil), // The initial bill has no stateHash defined
	).Return(nil)

	// The dust collector money gets set
	mockTree.On("Set", dustCollectorMoneySupplyID, tree.Predicate{},
		&BillData{V: dcMoneyAmount, T: 0, Backlink: nil},
		[]byte(nil), // The initial bill has no stateHash defined
	).Return(nil)

	_, err := NewMoneySchemeState(initialBill, dcMoneyAmount, MoneySchemeOpts.UnitsTree(mockTree))
	require.NoError(t, err)
}

func TestProcessTransfer(t *testing.T) {
	transferOk := newRandomTransfer()
	splitOk := newRandomSplit()
	testData := []struct {
		name        string
		transaction GenericTransaction
		expect      func(rs *MockRevertibleState)
		expectErr   error
	}{
		{
			name:        "transfer ok",
			transaction: transferOk,
			expect: func(rs *MockRevertibleState) {
				rs.On("SetOwner",
					transferOk.unitId,
					tree.Predicate(transferOk.newBearer),
					transferOk.Hash(crypto.SHA256),
				).Return(nil)
			},
			expectErr: nil,
		},
		{
			name:        "split ok",
			transaction: splitOk,
			expect: func(rs *MockRevertibleState) {
				var newGenericData tree.Data
				oldBillData := &BillData{
					V:        100,
					T:        0,
					Backlink: nil,
				}
				rs.On("UpdateData", splitOk.unitId, mock.Anything, splitOk.Hash(crypto.SHA256)).Run(func(args mock.Arguments) {
					upFunc := args.Get(UpdateDataUpdateFunction).(UpdateFunction)
					newGenericData = upFunc(oldBillData)
					newBD, ok := newGenericData.(*BillData)
					require.True(t, ok, "returned data is not BillData")
					require.Equal(t, oldBillData.V-splitOk.amount, newBD.V)
				}).Return(nil)

				rs.On("AddItem", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					expectedNewId := PrndSh(splitOk.unitId, splitOk.HashPrndSh(crypto.SHA256))
					actualId := args.Get(addItemId).(*uint256.Int)
					require.Equal(t, expectedNewId, actualId)

					actualOwner := args.Get(addItemOwner).(tree.Predicate)
					require.Equal(t, tree.Predicate(splitOk.targetBearer), actualOwner)

					expectedNewItemData := &BillData{
						V:        splitOk.Amount(),
						T:        0,
						Backlink: nil,
					}
					actualData := args.Get(addItemData).(tree.Data)
					require.Equal(t, expectedNewItemData, actualData)
				}).Return(nil)
			},
			expectErr: nil,
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			mockTree := new(MockUnitsTree)
			mockRState := new(MockRevertibleState)
			mockTree.On("Set", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
			mss, err := NewMoneySchemeState(
				&InitialBill{},
				0,
				MoneySchemeOpts.UnitsTree(mockTree),
				MoneySchemeOpts.RevertibleState(mockRState),
			)
			require.NoError(t, err)
			// Finished setup

			tt.expect(mockRState)

			err = mss.Process(tt.transaction)
			require.NoError(t, err)
			mock.AssertExpectationsForObjects(t, mockRState)
		})
	}
}

func newRandomTransfer() *transfer {
	trns := &transfer{
		genericTx: genericTx{
			unitId:     uint256.NewInt(1),
			timeout:    2,
			ownerProof: []byte{3},
		},
		newBearer:   []byte{4},
		targetValue: 5,
		backlink:    []byte{6},
	}
	return trns
}

func newRandomSplit() *split {
	return &split{
		genericTx: genericTx{
			unitId:     uint256.NewInt(1),
			timeout:    2,
			ownerProof: []byte{3},
		},
		amount:         4,
		targetBearer:   []byte{5},
		remainingValue: 6,
		backlink:       []byte{7},
	}
}
