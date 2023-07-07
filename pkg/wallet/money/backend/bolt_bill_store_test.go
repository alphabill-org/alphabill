package backend

import (
	"path/filepath"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestBillStore_CanBeCreated(t *testing.T) {
	bs, err := createTestBillStore(t)
	require.NoError(t, err)
	require.NotNil(t, bs)
}

func TestBillStore_GetSetBlockNumber(t *testing.T) {
	bs, _ := createTestBillStore(t)

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
	bs, _ := createTestBillStore(t)
	ownerPredicate1 := getOwnerPredicate("0x000000000000000000000000000000000000000000000000000000000000000001")
	ownerPredicate2 := getOwnerPredicate("0x000000000000000000000000000000000000000000000000000000000000000002")

	// verify GetBills for unknown predicate returns no error
	billsOwner1, err := bs.Do().GetBills(ownerPredicate1)
	require.NoError(t, err)
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
	bills, err := bs.Do().GetBills(ownerPredicate1)
	require.NoError(t, err)
	require.Len(t, bills, 3)
	require.Equal(t, billsOwner1, bills)

	// get owner 2 bills by predicate
	bills, err = bs.Do().GetBills(ownerPredicate2)
	require.NoError(t, err)
	require.Len(t, bills, 3)
	require.Equal(t, billsOwner2, bills)

	// when bill owner changes
	b := billsOwner1[0]
	b.OwnerPredicate = ownerPredicate2
	err = bs.Do().SetBill(b, nil)
	require.NoError(t, err)

	// then secondary indexes are updated
	bills, err = bs.Do().GetBills(ownerPredicate2)
	require.NoError(t, err)
	require.Len(t, bills, 4)

	bills, err = bs.Do().GetBills(ownerPredicate1)
	require.NoError(t, err)
	require.Len(t, bills, 2)

	// test get bill for unknown onwer nok
	bills, err = bs.Do().GetBills([]byte{1, 2, 3, 4})
	require.NoError(t, err)
	require.Len(t, bills, 0)
}

func TestBillStore_DeleteBill(t *testing.T) {
	bs, _ := createTestBillStore(t)
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
	bills, err := bs.Do().GetBills(p1)
	require.NoError(t, err)
	require.Len(t, bills, 0)
}

func TestBillStore_DeleteExpiredBills(t *testing.T) {
	s, _ := createTestBillStore(t)
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
	bills, err := s.Do().GetBills(bearer)
	require.NoError(t, err)
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
	bs, _ := createTestBillStore(t)
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
	bs, _ := createTestBillStore(t)

	// verify GetSystemDescriptionRecords is empty
	sdrs, err := bs.Do().GetSystemDescriptionRecords()
	require.NoError(t, err)
	require.Nil(t, sdrs)

	// add system description records
	sdrs = []*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: []byte{0},
			T2Timeout:        2500,
			FeeCreditBill:    &genesis.FeeCreditBill{UnitId: []byte{2}, OwnerPredicate: []byte{3}},
		},
		{
			SystemIdentifier: []byte{1},
			T2Timeout:        2500,
			FeeCreditBill:    &genesis.FeeCreditBill{UnitId: []byte{2}, OwnerPredicate: []byte{3}},
		},
	}
	err = bs.Do().SetSystemDescriptionRecords(sdrs)
	require.NoError(t, err)

	actualSDRs, err := bs.Do().GetSystemDescriptionRecords()
	require.NoError(t, err)
	require.Equal(t, sdrs, actualSDRs)
}

func TestBillStore_GetSetDeleteDCMetadata(t *testing.T) {
	bs, _ := createTestBillStore(t)

	// store metadata
	nonce := test.RandomBytes(32)
	bId := test.RandomBytes(32)
	expectedDcMetadata := &DCMetadata{DCSum: 10, BillIdentifiers: [][]byte{bId}}
	err := bs.Do().SetDCMetadata(nonce, expectedDcMetadata)
	require.NoError(t, err)

	// verify real metadata is retrieved
	actualMetadata, err := bs.Do().GetDCMetadata(nonce)
	require.NoError(t, err)
	require.Equal(t, expectedDcMetadata, actualMetadata)

	// verify non-existing metadata is nil
	invalidMetadata, err := bs.Do().GetDCMetadata(test.RandomBytes(32))
	require.NoError(t, err)
	require.Nil(t, invalidMetadata)

	// verify metadata is nil after delete
	err = bs.Do().DeleteDCMetadata(nonce)
	require.NoError(t, err)
	deletedMetadata, err := bs.Do().GetDCMetadata(nonce)
	require.NoError(t, err)
	require.Nil(t, deletedMetadata)
}

func TestBillStore_StoreTxHistoryRecord(t *testing.T) {
	bs, _ := createTestBillStore(t)
	hash := test.RandomBytes(32)
	max := byte(10)
	for i := byte(1); i <= max; i++ {
		txHistoryRecord := &sdk.TxHistoryRecord{
			TxHash: test.RandomBytes(32),
			UnitID: []byte{i},
		}
		// store tx history record
		err := bs.Do().StoreTxHistoryRecord(hash, txHistoryRecord)
		require.NoError(t, err)
	}
	// verify tx history records are retrieved, two most recent records
	actualTxHistoryRecords, key, err := bs.Do().GetTxHistoryRecords(hash, nil, 2)
	require.NoError(t, err)
	require.Len(t, actualTxHistoryRecords, 2)
	require.EqualValues(t, actualTxHistoryRecords[0].UnitID, []byte{max})
	require.NotNil(t, key)

	// verify tx history records are retrieved, all records
	var allTxHistoryRecords []*sdk.TxHistoryRecord
	key = nil
	for {
		actualTxHistoryRecords, key, err = bs.Do().GetTxHistoryRecords(hash, key, 2)
		require.NoError(t, err)
		allTxHistoryRecords = append(allTxHistoryRecords, actualTxHistoryRecords...)
		if key == nil {
			break
		}
	}
	require.Len(t, allTxHistoryRecords, int(max))
}

func createTestBillStore(t *testing.T) (*boltBillStore, error) {
	dbFile := filepath.Join(t.TempDir(), BoltBillStoreFileName)
	return newBoltBillStore(dbFile)
}

func getOwnerPredicate(pubkey string) []byte {
	pubKey, _ := hexutil.Decode(pubkey)
	return script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey))
}

func newBillWithValueAndOwner(val uint64, ownerPredicate []byte) *Bill {
	id := uint256.NewInt(val)
	return &Bill{
		Id:             util.Uint256ToBytes(id),
		Value:          val,
		OwnerPredicate: ownerPredicate,
	}
}
