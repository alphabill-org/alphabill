package backend

import (
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/common/hash"
	"github.com/alphabill-org/alphabill/common/util"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/api/types"
	sdk "github.com/alphabill-org/alphabill/client/wallet"
	"github.com/alphabill-org/alphabill/validator/pkg/network/protocol/genesis"
	test "github.com/alphabill-org/alphabill/validator/pkg/testutils"
	testtransaction "github.com/alphabill-org/alphabill/validator/pkg/testutils/transaction"
)

func TestBillStore_CanBeCreated(t *testing.T) {
	bs := createTestBillStore(t)
	require.NotNil(t, bs)
}

func TestBillStore_GetSetBlockNumber(t *testing.T) {
	bs := createTestBillStore(t)

	// verify initial block number is 0
	blockNumber, err := bs.Do().GetBlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 0, blockNumber)

	// set block number
	err = bs.Do().SetBlockNumber(1)
	require.NoError(t, err)

	// verify block number
	blockNumber, err = bs.Do().GetBlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 1, blockNumber)
}

func TestBillStore_GetSetBills(t *testing.T) {
	bs := createTestBillStore(t)
	ownerPredicate1 := getOwnerPredicate("0x000000000000000000000000000000000000000000000000000000000000000001")
	ownerPredicate2 := getOwnerPredicate("0x000000000000000000000000000000000000000000000000000000000000000002")

	// verify GetBills for unknown predicate returns no error
	billsOwner1, nextKey, err := bs.Do().GetBills(ownerPredicate1, true, nil, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, billsOwner1, 0)

	// add bills for owner 1
	billsOwner1 = []*Bill{
		newBillWithValueAndOwner(1, ownerPredicate1),
		newBillWithValueAndOwner(2, ownerPredicate1),
		newBillWithValueAndOwner(3, ownerPredicate1),
	}
	for _, b := range billsOwner1 {
		err = bs.Do().SetBill(b, nil)
		require.NoError(t, err)
	}

	// add bills for owner 2
	billsOwner2 := []*Bill{
		newBillWithValueAndOwner(4, ownerPredicate2),
		newBillWithValueAndOwner(5, ownerPredicate2),
		newBillWithValueAndOwner(6, ownerPredicate2),
	}
	for _, b := range billsOwner2 {
		err = bs.Do().SetBill(b, nil)
		require.NoError(t, err)
	}

	// get owner 1 bills by ID
	for _, expectedBill := range billsOwner1 {
		actualBill, err := bs.Do().GetBill(expectedBill.Id)
		require.NoError(t, err)
		require.Equal(t, expectedBill, actualBill)
	}

	// get owner 2 bills by ID
	for _, expectedBill := range billsOwner2 {
		actualBill, err := bs.Do().GetBill(expectedBill.Id)
		require.NoError(t, err)
		require.Equal(t, expectedBill, actualBill)
	}

	// get owner 1 bills by predicate
	bills, nextKey, err := bs.Do().GetBills(ownerPredicate1, true, nil, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, bills, 3)
	require.Equal(t, billsOwner1, bills)

	// get owner 2 bills by predicate
	bills, nextKey, err = bs.Do().GetBills(ownerPredicate2, true, nil, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, bills, 3)
	require.Equal(t, billsOwner2, bills)

	// when bill owner changes
	b := billsOwner1[0]
	b.OwnerPredicate = ownerPredicate2
	err = bs.Do().SetBill(b, nil)
	require.NoError(t, err)

	// then secondary indexes are updated
	bills, nextKey, err = bs.Do().GetBills(ownerPredicate2, true, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 4)
	require.Nil(t, nextKey)

	bills, nextKey, err = bs.Do().GetBills(ownerPredicate1, true, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 2)
	require.Nil(t, nextKey)

	// test get bill for unknown owner nok
	bills, nextKey, err = bs.Do().GetBills([]byte{1, 2, 3, 4}, true, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 0)
	require.Nil(t, nextKey)
}

func TestBillStore_DeleteBill(t *testing.T) {
	bs := createTestBillStore(t)
	p1 := getOwnerPredicate("0x000000000000000000000000000000000000000000000000000000000000000001")

	// add bill
	bill := newBillWithValueAndOwner(1, p1)
	err := bs.Do().SetBill(bill, nil)
	require.NoError(t, err)

	// verify bill is added
	b, err := bs.Do().GetBill(bill.Id)
	require.NoError(t, err)
	require.Equal(t, bill, b)

	// when bill is removed
	err = bs.Do().RemoveBill(bill.Id)
	require.NoError(t, err)

	// then bill should be deleted from main store
	b, err = bs.Do().GetBill(bill.Id)
	require.NoError(t, err)
	require.Nil(t, b)

	// and from predicate index
	bills, nextKey, err := bs.Do().GetBills(p1, true, nil, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, bills, 0)
}

func TestBillStore_DeleteExpiredBills(t *testing.T) {
	s := createTestBillStore(t)
	bearer := getOwnerPredicate(pubkeyHex)
	expirationBlockNo := uint64(100)
	unitIDs := [][]byte{{1}, {2}, {3}}

	// add three bills and set expiration time
	for _, unitID := range unitIDs {
		err := s.Do().SetBill(&Bill{Id: unitID, OwnerPredicate: bearer}, nil)
		require.NoError(t, err)

		err = s.Do().SetBillExpirationTime(expirationBlockNo, unitID)
		require.NoError(t, err)
	}

	// when expiration time is reached
	err := s.Do().DeleteExpiredBills(expirationBlockNo)
	require.NoError(t, err)

	// then expired bills should be deleted
	bills, nextKey, err := s.Do().GetBills(bearer, true, nil, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, bills, 0)

	// and metadata should also be deleted
	err = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(expiredBillsBucket).Get(util.Uint64ToBytes(expirationBlockNo))
		require.Nil(t, b)
		return nil
	})
	require.NoError(t, err)
}

func TestBillStore_GetSetFeeCreditBills(t *testing.T) {
	bs := createTestBillStore(t)
	ownerPredicate := getOwnerPredicate("0x000000000000000000000000000000000000000000000000000000000000000001")
	fcbID := []byte{0}

	// verify GetFeeCreditBill for unknown id returns no error
	fcb, err := bs.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.Nil(t, fcb)

	// add fee credit bills
	fcbs := []*Bill{
		newBillWithValueAndOwner(1, ownerPredicate),
		newBillWithValueAndOwner(2, ownerPredicate),
		newBillWithValueAndOwner(3, ownerPredicate),
	}
	for _, b := range fcbs {
		err = bs.Do().SetFeeCreditBill(b, nil)
		require.NoError(t, err)
	}

	// verify GetFeeCreditBill is not nil
	for _, expectedFCB := range fcbs {
		actualFCB, err := bs.Do().GetFeeCreditBill(expectedFCB.Id)
		require.NoError(t, err)
		require.Equal(t, expectedFCB, actualFCB)
	}
}

func TestBillStore_GetSetSystemDescriptionRecordsBills(t *testing.T) {
	bs := createTestBillStore(t)

	// verify GetSystemDescriptionRecords is empty
	sdrs, err := bs.Do().GetSystemDescriptionRecords()
	require.NoError(t, err)
	require.Nil(t, sdrs)

	// add system description records
	sdrs = []*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: []byte{0},
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         []byte{2},
				OwnerPredicate: []byte{3},
			},
		},
		{
			SystemIdentifier: []byte{1},
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         []byte{2},
				OwnerPredicate: []byte{3},
			},
		},
	}
	err = bs.Do().SetSystemDescriptionRecords(sdrs)
	require.NoError(t, err)

	actualSDRs, err := bs.Do().GetSystemDescriptionRecords()
	require.NoError(t, err)
	require.Equal(t, sdrs, actualSDRs)
}

func TestBillStore_GetSetLockedFeeCredit(t *testing.T) {
	bs := createTestBillStore(t)
	systemID := []byte{0, 0, 0, 0}
	fcbID := money.NewFeeCreditRecordID(nil, []byte{1})

	// verify GetLockedFeeCredit no result returns no error
	lfc, err := bs.Do().GetLockedFeeCredit(systemID, fcbID)
	require.NoError(t, err)
	require.Nil(t, lfc)

	// add locked fee credit
	transferFC := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	err = bs.Do().SetLockedFeeCredit(systemID, fcbID, transferFC)
	require.NoError(t, err)

	// verify GetFeeCreditBill is not nil
	lfc, err = bs.Do().GetLockedFeeCredit(systemID, fcbID)
	require.NoError(t, err)
	require.Equal(t, lfc, transferFC)
}

func TestBillStore_GetSetClosedFeeCredit(t *testing.T) {
	bs := createTestBillStore(t)
	systemID := []byte{0, 0, 0, 0}
	fcbID := money.NewFeeCreditRecordID(nil, []byte{1})

	// verify GetLockedFeeCredit no result returns no error
	lfc, err := bs.Do().GetLockedFeeCredit(systemID, fcbID)
	require.NoError(t, err)
	require.Nil(t, lfc)

	// add locked fee credit
	transferFC := &types.TransactionRecord{
		TransactionOrder: testutils.NewTransferFC(t, nil),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	err = bs.Do().SetLockedFeeCredit(systemID, fcbID, transferFC)
	require.NoError(t, err)

	// verify GetFeeCreditBill is not nil
	lfc, err = bs.Do().GetLockedFeeCredit(systemID, fcbID)
	require.NoError(t, err)
	require.Equal(t, lfc, transferFC)
}

func TestBillStore_StoreTxHistoryRecord(t *testing.T) {
	bs := createTestBillStore(t)
	randomBytes := test.RandomBytes(32)
	max := byte(10)
	for i := byte(1); i <= max; i++ {
		txHistoryRecord := &sdk.TxHistoryRecord{
			TxHash: test.RandomBytes(32),
			UnitID: []byte{i},
		}
		// store tx history record
		err := bs.Do().StoreTxHistoryRecord(randomBytes, txHistoryRecord)
		require.NoError(t, err)
	}
	// verify tx history records are retrieved, two most recent records
	actualTxHistoryRecords, key, err := bs.Do().GetTxHistoryRecords(randomBytes, nil, 2)
	require.NoError(t, err)
	require.Len(t, actualTxHistoryRecords, 2)
	require.EqualValues(t, actualTxHistoryRecords[0].UnitID, []byte{max})
	require.NotNil(t, key)

	// verify tx history records are retrieved, all records
	var allTxHistoryRecords []*sdk.TxHistoryRecord
	key = nil
	for {
		actualTxHistoryRecords, key, err = bs.Do().GetTxHistoryRecords(randomBytes, key, 2)
		require.NoError(t, err)
		allTxHistoryRecords = append(allTxHistoryRecords, actualTxHistoryRecords...)
		if key == nil {
			break
		}
	}
	require.Len(t, allTxHistoryRecords, int(max))
}

func TestBillStore_Paging(t *testing.T) {
	bs := createTestBillStore(t)
	pubKey := "0x000000000000000000000000000000000000000000000000000000000000000001"
	ownerPredicate := getOwnerPredicate(pubKey)

	// add bills
	var bills []*Bill
	for i := byte(0); i < 10; i++ {
		b := newBillWithValueAndOwner(i, ownerPredicate)
		bills = append(bills, b)

		err := bs.Do().SetBill(b, nil)
		require.NoError(t, err)
	}

	// get all bills
	actualBills, nextKey, err := bs.Do().GetBills(ownerPredicate, true, nil, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, actualBills, 10)
	require.Equal(t, bills, actualBills)

	// get first 5 bills
	actualBills, nextKey, err = bs.Do().GetBills(ownerPredicate, true, nil, 5)
	require.NoError(t, err)
	require.Len(t, actualBills, 5)
	require.Equal(t, bills[:5], actualBills)
	require.Equal(t, bills[5].Id, nextKey)

	// get 5 bills starting from second bill
	actualBills, nextKey, err = bs.Do().GetBills(ownerPredicate, true, bills[1].Id, 5)
	require.NoError(t, err)
	require.Len(t, actualBills, 5)
	require.Equal(t, bills[1:6], actualBills)
	require.Equal(t, bills[6].Id, nextKey)

	// get 5 bills starting from second to last bill
	actualBills, nextKey, err = bs.Do().GetBills(ownerPredicate, true, bills[8].Id, 5)
	require.NoError(t, err)
	require.Len(t, actualBills, 2)
	require.Nil(t, nextKey)
	require.Equal(t, bills[8:], actualBills)
}

func TestBillStore_Paging_SeekToInvalidKey(t *testing.T) {
	bs := createTestBillStore(t)
	pubKey := "0x000000000000000000000000000000000000000000000000000000000000000001"
	ownerPredicate := getOwnerPredicate(pubKey)

	// add bills with ID 5
	b := newBillWithValueAndOwner(5, ownerPredicate)
	err := bs.Do().SetBill(b, nil)
	require.NoError(t, err)

	// verify that starting from ID 4 does not return bill with ID 5
	invalidID := util.Uint64ToBytes32(4)
	bills, nextKey, err := bs.Do().GetBills(ownerPredicate, true, invalidID, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, bills, 0)
}

func TestBillStore_Paging_FilterDCBills(t *testing.T) {
	bs := createTestBillStore(t)
	pubKey := "0x000000000000000000000000000000000000000000000000000000000000000001"
	ownerPredicate := getOwnerPredicate(pubKey)

	// add bills
	for i := byte(0); i < 10; i++ {
		b := newBillWithValueAndOwner(i, ownerPredicate)
		b.DCTargetUnitID = test.RandomBytes(32)
		b.DCTargetUnitBacklink = test.RandomBytes(32)
		err := bs.Do().SetBill(b, nil)
		require.NoError(t, err)
	}

	// get all bills
	actualBills, nextKey, err := bs.Do().GetBills(ownerPredicate, false, nil, 100)
	require.NoError(t, err)
	require.Len(t, actualBills, 0)
	require.Nil(t, nextKey)
}

func TestBillStore_StoreTxProof(t *testing.T) {
	bs := createTestBillStore(t)
	unitID := types.UnitID(test.RandomBytes(33))
	txHash := test.RandomBytes(32)

	// store tx proof
	err := bs.Do().StoreTxProof(unitID, txHash, &sdk.Proof{
		TxRecord: testtransaction.NewTransactionRecord(t, testtransaction.WithUnitId(unitID)),
	})
	require.NoError(t, err)

	// verify stored tx proof can be retrieved
	actualProof, err := bs.Do().GetTxProof(unitID, txHash)
	require.NoError(t, err)
	require.Equal(t, unitID, actualProof.TxRecord.TransactionOrder.UnitID())
}

func createTestBillStore(t *testing.T) *boltBillStore {
	dbFile := filepath.Join(t.TempDir(), BoltBillStoreFileName)
	store, err := newBoltBillStore(dbFile)
	require.NoError(t, err)
	return store
}

func getOwnerPredicate(pubkey string) []byte {
	pubKey, _ := hexutil.Decode(pubkey)
	return templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey))
}

func newBillWithValueAndOwner(val byte, ownerPredicate []byte) *Bill {
	return &Bill{
		Id:             newBillID(val),
		Value:          uint64(val),
		OwnerPredicate: ownerPredicate,
	}
}
