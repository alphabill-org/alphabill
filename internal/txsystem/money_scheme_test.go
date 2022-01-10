package txsystem

import (
	"crypto"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

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
	mockRevertibleState := new(MockRevertibleState)

	initialBill := &InitialBill{ID: uint256.NewInt(2), Value: 100, Owner: nil}
	dcMoneyAmount := uint64(222)

	// Initial bill gets set
	mockRevertibleState.On("AddItem", initialBill.ID, initialBill.Owner,
		&BillData{V: initialBill.Value, T: 0, Backlink: nil},
		[]byte(nil), // The initial bill has no stateHash defined
	).Return(nil)

	// The dust collector money gets set
	mockRevertibleState.On("AddItem", dustCollectorMoneySupplyID, state.Predicate{},
		&BillData{V: dcMoneyAmount, T: 0, Backlink: nil},
		[]byte(nil), // The initial bill has no stateHash defined
	).Return(nil)

	_, err := NewMoneySchemeState(crypto.SHA256, initialBill, dcMoneyAmount, MoneySchemeOpts.RevertibleState(mockRevertibleState))
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
					state.Predicate(transferOk.newBearer),
					transferOk.Hash(crypto.SHA256),
				).Return(nil)
			},
			expectErr: nil,
		},
		{
			name:        "split ok",
			transaction: splitOk,
			expect: func(rs *MockRevertibleState) {
				var newGenericData state.UnitData
				oldBillData := &BillData{
					V:        100,
					T:        0,
					Backlink: nil,
				}
				rs.On("UpdateData", splitOk.unitId, mock.Anything, splitOk.Hash(crypto.SHA256)).Run(func(args mock.Arguments) {
					upFunc := args.Get(UpdateDataUpdateFunction).(state.UpdateFunction)
					newGenericData = upFunc(oldBillData)
					newBD, ok := newGenericData.(*BillData)
					require.True(t, ok, "returned data is not BillData")
					require.Equal(t, oldBillData.V-splitOk.amount, newBD.V)
				}).Return(nil)

				rs.On("AddItem", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					expectedNewId := PrndSh(splitOk.unitId, splitOk.HashPrndSh(crypto.SHA256))
					actualId := args.Get(addItemId).(*uint256.Int)
					require.Equal(t, expectedNewId, actualId)

					actualOwner := args.Get(addItemOwner).(state.Predicate)
					require.Equal(t, state.Predicate(splitOk.targetBearer), actualOwner)

					expectedNewItemData := &BillData{
						V:        splitOk.Amount(),
						T:        0,
						Backlink: nil,
					}
					actualData := args.Get(addItemData).(state.UnitData)
					require.Equal(t, expectedNewItemData, actualData)
				}).Return(nil)
			},
			expectErr: nil,
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			mockRState := new(MockRevertibleState)
			initialBill := &InitialBill{ID: uint256.NewInt(77), Value: 10, Owner: state.Predicate{44}}
			mockRState.On("AddItem", initialBill.ID, initialBill.Owner, mock.Anything, mock.Anything).Return(nil)
			mockRState.On("AddItem", dustCollectorMoneySupplyID, state.Predicate{}, mock.Anything, mock.Anything).Return(nil)
			mss, err := NewMoneySchemeState(
				crypto.SHA256,
				initialBill,
				0,
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

func TestBillData_Value(t *testing.T) {
	bd := &BillData{
		V:        10,
		T:        0,
		Backlink: nil,
	}

	actualSumValue := bd.Value()
	expectedSumValue := &BillSummary{v: 10}
	require.Equal(t, expectedSumValue, actualSumValue)
}

func TestBillData_AddToHasher(t *testing.T) {
	bd := &BillData{
		V:        10,
		T:        50,
		Backlink: []byte("backlink"),
	}

	hasher := crypto.SHA256.New()
	hasher.Write(util.Uint64ToBytes(bd.V))
	hasher.Write(util.Uint64ToBytes(bd.T))
	hasher.Write(bd.Backlink)
	expectedHash := hasher.Sum(nil)
	hasher.Reset()
	bd.AddToHasher(hasher)
	actualHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
}

func TestBillSummary_Concatenate(t *testing.T) {
	self := &BillSummary{v: 10}
	left := &BillSummary{v: 2}
	right := &BillSummary{v: 3}

	actualSum := self.Concatenate(left, right)
	require.Equal(t, &BillSummary{v: 15}, actualSum)

	actualSum = self.Concatenate(nil, nil)
	require.Equal(t, &BillSummary{v: 10}, actualSum)

	actualSum = self.Concatenate(left, nil)
	require.Equal(t, &BillSummary{v: 12}, actualSum)

	actualSum = self.Concatenate(nil, right)
	require.Equal(t, &BillSummary{v: 13}, actualSum)
}

func TestBillSummary_AddToHasher(t *testing.T) {
	bs := &BillSummary{v: 10}

	hasher := crypto.SHA256.New()
	hasher.Write(util.Uint64ToBytes(bs.v))
	expectedHash := hasher.Sum(nil)
	hasher.Reset()

	bs.AddToHasher(hasher)
	actualHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
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
