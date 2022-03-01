package txsystem

import (
	"crypto"
	"math/rand"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/mocks"

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
	mockRevertibleState := new(mocks.RevertibleState)

	initialBill := &InitialBill{ID: uint256.NewInt(2), Value: 100, Owner: nil}
	dcMoneyAmount := uint64(222)

	// Initial bill gets set
	mockRevertibleState.On("AddItem", initialBill.ID, initialBill.Owner,
		&BillData{V: initialBill.Value, T: 0, Backlink: nil},
		[]byte(nil), // The initial bill has no stateHash defined
	).Return(nil)

	// The dust collector money gets set
	mockRevertibleState.On("AddItem", dustCollectorMoneySupplyID, state.Predicate(dustCollectorPredicate),
		&BillData{V: dcMoneyAmount, T: 0, Backlink: nil},
		[]byte(nil), // The initial bill has no stateHash defined
	).Return(nil)

	_, err := NewMoneySchemeState(crypto.SHA256, initialBill, dcMoneyAmount, MoneySchemeOpts.RevertibleState(mockRevertibleState))
	require.NoError(t, err)
}

func TestNewMoneyScheme_InitialBillIsNil(t *testing.T) {
	_, err := NewMoneySchemeState(crypto.SHA256, nil, 10)
	require.ErrorIs(t, err, ErrInitialBillIsNil)
}

func TestNewMoneyScheme_InvalidInitialBillID(t *testing.T) {
	initialBill := &InitialBill{ID: uint256.NewInt(0), Value: 100, Owner: nil}
	_, err := NewMoneySchemeState(crypto.SHA256, initialBill, 10)
	require.ErrorIs(t, err, ErrInvalidInitialBillID)
}

func TestProcessTransaction(t *testing.T) {
	transferOk := newRandomTransfer()
	splitOk := newRandomSplit()
	transferDCOk := newRandomTransferDC()
	blockNumber := uint64(0)
	testData := []struct {
		name        string
		transaction GenericTransaction
		expect      func(rs *mocks.RevertibleState)
		expectErr   error
	}{
		{
			name:        "transfer ok",
			transaction: transferOk,
			expect: func(rs *mocks.RevertibleState) {
				rs.On("GetBlockNumber").Return(blockNumber)
				rs.On("GetUnit", transferOk.UnitId()).Return(&state.Unit{Data: &BillData{V: transferOk.targetValue, Backlink: transferOk.backlink}}, nil)
				rs.On("UpdateData", transferOk.unitId, mock.Anything, transferOk.Hash(crypto.SHA256)).Run(func(args mock.Arguments) {
					upFunc := args.Get(UpdateDataUpdateFunction).(state.UpdateFunction)
					oldBillData := &BillData{
						V:        5,
						T:        0,
						Backlink: nil,
					}
					newUnitData := upFunc(oldBillData)
					newBD, ok := newUnitData.(*BillData)
					require.True(t, ok, "returned data is not BillData")
					require.EqualValues(t, transferOk.Hash(crypto.SHA256), newBD.Backlink)
					require.Equal(t, blockNumber, newBD.T)
				}).Return(nil)
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
			expect: func(rs *mocks.RevertibleState) {
				var newGenericData state.UnitData
				oldBillData := &BillData{
					V:        10,
					T:        0,
					Backlink: splitOk.Backlink(),
				}
				rs.On("GetBlockNumber").Return(blockNumber)
				rs.On("GetUnit", splitOk.UnitId()).Return(&state.Unit{Data: oldBillData}, nil)
				rs.On("UpdateData", splitOk.unitId, mock.Anything, splitOk.Hash(crypto.SHA256)).Run(func(args mock.Arguments) {
					upFunc := args.Get(UpdateDataUpdateFunction).(state.UpdateFunction)
					newGenericData = upFunc(oldBillData)
					newBD, ok := newGenericData.(*BillData)
					require.True(t, ok, "returned data is not BillData")
					require.Equal(t, oldBillData.V-splitOk.amount, newBD.V)
				}).Return(nil)

				rs.On("AddItem", mock.Anything, mock.Anything, mock.Anything, splitOk.Hash(crypto.SHA256)).Run(func(args mock.Arguments) {
					expectedNewId := sameShardId(splitOk.unitId, splitOk.HashForIdCalculation(crypto.SHA256))
					actualId := args.Get(addItemId).(*uint256.Int)
					require.Equal(t, expectedNewId, actualId)

					actualOwner := args.Get(addItemOwner).(state.Predicate)
					require.Equal(t, state.Predicate(splitOk.targetBearer), actualOwner)

					expectedNewItemData := &BillData{
						V:        splitOk.Amount(),
						T:        0,
						Backlink: splitOk.Hash(crypto.SHA256),
					}
					actualData := args.Get(addItemData).(state.UnitData)
					require.Equal(t, expectedNewItemData, actualData)
				}).Return(nil)
			},
			expectErr: nil,
		},
		{
			name:        "transferDC ok",
			transaction: transferDCOk,
			expect: func(rs *mocks.RevertibleState) {
				rs.On("SetOwner",
					transferDCOk.unitId,
					state.Predicate(dustCollectorPredicate),
					transferDCOk.Hash(crypto.SHA256),
				).Return(nil)

				rs.On("GetBlockNumber").Return(blockNumber)
				rs.On("GetUnit", transferDCOk.UnitId()).Return(&state.Unit{Data: &BillData{V: transferDCOk.targetValue, Backlink: transferDCOk.backlink}}, nil)
				rs.On("UpdateData", transferDCOk.unitId, mock.Anything, transferDCOk.Hash(crypto.SHA256)).Run(func(args mock.Arguments) {
					upFunc := args.Get(UpdateDataUpdateFunction).(state.UpdateFunction)
					oldBillData := &BillData{
						V:        5,
						T:        0,
						Backlink: nil,
					}
					newUnitData := upFunc(oldBillData)
					newBD, ok := newUnitData.(*BillData)
					require.True(t, ok, "returned data is not BillData")
					require.EqualValues(t, transferDCOk.Hash(crypto.SHA256), newBD.Backlink)
					require.Equal(t, blockNumber, newBD.T)
				}).Return(nil)

			},
			expectErr: nil,
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			mockRState := new(mocks.RevertibleState)
			initialBill := &InitialBill{ID: uint256.NewInt(77), Value: 10, Owner: state.Predicate{44}}
			mockRState.On("AddItem", initialBill.ID, initialBill.Owner, mock.Anything, mock.Anything).Return(nil)
			mockRState.On("AddItem", dustCollectorMoneySupplyID, state.Predicate(dustCollectorPredicate), mock.Anything, mock.Anything).Return(nil)
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

func TestEndBlock_DustBillsAreRemoved(t *testing.T) {
	mockRState := new(mocks.RevertibleState)
	mss, err := NewMoneyScheme(mockRState)
	require.NoError(t, err)

	var currentBlock uint64 = 10
	transferDCTxCount := 5

	var transactions []*transferDC
	// process transactions

	for i := 0; i < transferDCTxCount; i++ {
		transferDC := newRandomTransferDC()
		transferDC.timeout = 100
		mockRState.On("SetOwner", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		mockRState.On("GetBlockNumber").Return(currentBlock)
		mockRState.On("GetUnit", mock.Anything).Return(&state.Unit{Data: &BillData{V: transferDC.targetValue, Backlink: transferDC.backlink}}, nil)
		mockRState.On("UpdateData", transferDC.unitId, mock.Anything, mock.Anything).Return(nil)
		err = mss.Process(transferDC)
		require.NoError(t, err)
		transactions = append(transactions, transferDC)
	}
	require.Equal(t, 0, len(mss.dustCollectorBills[currentBlock]))
	delBlockNr := currentBlock + dustBillDeletionTimeout
	require.Equal(t, transferDCTxCount, len(mss.dustCollectorBills[delBlockNr]))

	// reset mocks
	mockRState = new(mocks.RevertibleState)
	mss.revertibleState = mockRState

	// EndBlock mocks
	var totalDustAmount uint64 = 0
	for i, tx := range transactions {
		dustAmount := uint64(10 + i)
		mockRState.On("GetUnit", tx.unitId).Return(&state.Unit{
			Bearer: nil,
			Data: &BillData{
				V:        dustAmount,
				T:        0,
				Backlink: []byte{1},
			},
			StateHash: nil,
		}, nil)
		totalDustAmount += dustAmount
		mockRState.On("DeleteItem", tx.unitId).Return(nil)
	}

	// DustCollector money supply update mock
	mockRState.On("UpdateData", dustCollectorMoneySupplyID, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		upFunc := args.Get(UpdateDataUpdateFunction).(state.UpdateFunction)
		var oldValue uint64 = 100
		dustCollectorBillData := &BillData{
			V:        oldValue,
			T:        0,
			Backlink: nil,
		}
		newGenericData := upFunc(dustCollectorBillData)
		newBD, ok := newGenericData.(*BillData)
		require.True(t, ok, "returned data is not BillData")
		require.Equal(t, oldValue+totalDustAmount, newBD.V)
	}).Return(nil)

	err = mss.EndBlock(delBlockNr)
	require.NoError(t, err)
	mock.AssertExpectationsForObjects(t, mockRState)
}

func NewMoneyScheme(mockRState *mocks.RevertibleState) (*moneySchemeState, error) {
	initialBill := &InitialBill{ID: uint256.NewInt(77), Value: 10, Owner: state.Predicate{44}}
	mockRState.On("AddItem", initialBill.ID, initialBill.Owner, mock.Anything, mock.Anything).Return(nil)
	mockRState.On("AddItem", dustCollectorMoneySupplyID, state.Predicate(dustCollectorPredicate), mock.Anything, mock.Anything).Return(nil)
	mss, err := NewMoneySchemeState(
		crypto.SHA256,
		initialBill,
		100,
		MoneySchemeOpts.RevertibleState(mockRState),
	)
	return mss, err
}

func newRandomTransfer() *transfer {
	trns := &transfer{
		genericTx: genericTx{
			systemID:   []byte{0},
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

func newRandomTransferDC() *transferDC {
	trns := &transferDC{
		genericTx: genericTx{
			systemID:   []byte{0},
			unitId:     uint256.NewInt(rand.Uint64()),
			timeout:    2,
			ownerProof: []byte{3},
		},
		targetBearer: []byte{4},
		targetValue:  5,
		backlink:     []byte{6},
		nonce:        []byte{7},
	}
	return trns
}

func newRandomSplit() *split {
	return &split{
		genericTx: genericTx{
			systemID:   []byte{0},
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
