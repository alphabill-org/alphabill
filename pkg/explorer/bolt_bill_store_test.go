package explorer

import (
	"crypto"
	"encoding/hex"

	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestBillStore_CanBeCreated(t *testing.T) {
	bs := createTestBillStore(t)
	require.NotNil(t, bs)
}

func TestBillStore_SetBlock_GetBlockByBlockNumber(t *testing.T) {
	blockNumber := test.RandomUint64()
	bs := createTestBillStore(t)
	b := &types.Block{Header: &types.Header{}, UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber}}}

	// set block
	err := bs.Do().SetBlock(b)
	require.NoError(t, err)

	// get block
	block, err := bs.Do().GetBlockByBlockNumber(blockNumber)
	require.NoError(t, err)
	require.EqualValues(t, block.UnicityCertificate.InputRecord.RoundNumber, blockNumber)
}
func TestBillStore_SetBlocks_GetBlocks(t *testing.T) {
	blockNumber1 := test.RandomUint64()
	blockNumber2 := blockNumber1 + 1
	blockNumber3 := blockNumber2 + 1

	bs := createTestBillStore(t)
	b1 := &types.Block{Header: &types.Header{}, UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber1}}}
	b2 := &types.Block{Header: &types.Header{}, UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber2}}}
	b3 := &types.Block{Header: &types.Header{}, UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber3}}}

	// set blocks
	err := bs.Do().SetBlock(b1)
	require.NoError(t, err)
	err = bs.Do().SetBlock(b2)
	require.NoError(t, err)
	err = bs.Do().SetBlock(b3)
	require.NoError(t, err)

	// get blocks
	length := 2
	blocks, prevBlockNumber, err := bs.Do().GetBlocks(blockNumber3, length)
	require.NoError(t, err)
	require.EqualValues(t, blocks[0].UnicityCertificate.InputRecord.RoundNumber, blockNumber3)
	require.EqualValues(t, prevBlockNumber, blockNumber1)
	require.EqualValues(t, len(blocks), length)

	length2 := 4
	blocks2, prevBlockNumber2, err2 := bs.Do().GetBlocks(blockNumber3, length2)
	require.NoError(t, err2)
	require.EqualValues(t, blocks2[0].UnicityCertificate.InputRecord.RoundNumber, blockNumber3)
	require.EqualValues(t, prevBlockNumber2, 0)
	require.EqualValues(t, len(blocks2), 3)
}
func TestBillStore_SetBlockExplorer_GetBlockExplorerByBlockNumber(t *testing.T) {
	blockNumber := test.RandomUint64()
	bs := createTestBillStore(t)
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 10, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}
	b := &types.Block{Header: &types.Header{ } , Transactions:  []*types.TransactionRecord{tx} , UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber} , UnicitySeal: &types.UnicitySeal{}}}
	txHash := hex.EncodeToString(tx.Hash(crypto.SHA256))
	// set block explorer
	err := bs.Do().SetBlockExplorer(b)
	require.NoError(t, err)

	// get block explorer
	block, err := bs.Do().GetBlockExplorerByBlockNumber(blockNumber)
	require.NoError(t, err)
	require.EqualValues(t, block.RoundNumber, blockNumber)
	require.EqualValues(t, block.TxHashes[0] , txHash)
}
func TestBillStore_SetTxExplorerToBucket_GetTxExplorerByTxHash(t *testing.T) {
	blockNumber := test.RandomUint64()
	bs := createTestBillStore(t)
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 10, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}

	// Set TxExplorer To Bucket
	txExplorer ,err := CreateTxExplorer(blockNumber , tx);
	require.NoError(t, err);
	err = bs.Do().SetTxExplorerToBucket(txExplorer)
	require.NoError(t, err)

	// Get TxExplorer By TxHash
	//hashHex := hex.EncodeToString(tx.Hash(crypto.SHA256))
	hash := hex.EncodeToString(tx.Hash(crypto.SHA256));
	txExplorerResult, err := bs.Do().GetTxExplorerByTxHash(hash);
	require.NoError(t, err)
	require.EqualValues(t, txExplorerResult.BlockNumber, blockNumber)
	require.EqualValues(t, txExplorerResult.Hash , hash)
	require.EqualValues(t, txExplorerResult.Fee , tx.ServerMetadata.ActualFee)
}
func TestBillStore_SetBlockExplorer_GetBlocksExplorer(t *testing.T) {
	blockNumber1 := test.RandomUint64()
	blockNumber2 := blockNumber1 + 1
	blockNumber3 := blockNumber2 + 1

	tx1 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 10, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}
	tx2 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 50, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}
	tx3 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1111, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}
	bs := createTestBillStore(t)
	b1 := &types.Block{Header: &types.Header{ } , Transactions:  []*types.TransactionRecord{tx1} , UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber1} , UnicitySeal: &types.UnicitySeal{}}}
	b2 := &types.Block{Header: &types.Header{ } , Transactions:  []*types.TransactionRecord{tx2} , UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber2} , UnicitySeal: &types.UnicitySeal{}}}
	b3 := &types.Block{Header: &types.Header{ } , Transactions:  []*types.TransactionRecord{tx3} , UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber3} , UnicitySeal: &types.UnicitySeal{}}}

	tx1Hash:= hex.EncodeToString(tx1.Hash(crypto.SHA256));
	//tx2Hash:= hex.EncodeToString(tx2.Hash(crypto.SHA256));
	tx3Hash:= hex.EncodeToString(tx3.Hash(crypto.SHA256));
	// set blocks
	err := bs.Do().SetBlockExplorer(b1)
	require.NoError(t, err)
	err = bs.Do().SetBlockExplorer(b2)
	require.NoError(t, err)
	err = bs.Do().SetBlockExplorer(b3)
	require.NoError(t, err)

	// get blocks
	length := 2
	blocks, prevBlockNumber, err := bs.Do().GetBlocksExplorer(blockNumber3, length)
	require.NoError(t, err)
	require.EqualValues(t, blocks[0].RoundNumber, blockNumber3)
	require.EqualValues(t, prevBlockNumber, blockNumber1)
	require.EqualValues(t, len(blocks), length)
	require.EqualValues(t, blocks[0].TxHashes[0] , tx3Hash)

	length2 := 4
	blocks2, prevBlockNumber2, err2 := bs.Do().GetBlocksExplorer(blockNumber3, length2)
	require.NoError(t, err2)
	require.EqualValues(t, blocks2[0].RoundNumber, blockNumber3)
	require.EqualValues(t, prevBlockNumber2, 0)
	require.EqualValues(t, len(blocks2), 3)
	require.EqualValues(t, blocks2[2].TxHashes[0] , tx1Hash)
}
func TestBillStore_SetBlockExplorer_GetBlockExplorerTxsByBlockNumber(t *testing.T) {
	blockNumber := test.RandomUint64()

	tx1 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 10, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}
	tx2 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 50, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}
	tx3 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{},
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1111, TargetUnits: []types.UnitID{}, SuccessIndicator: 0, ProcessingDetails: []byte{}},
	}
	bs := createTestBillStore(t)
	b := &types.Block{Header: &types.Header{ } , Transactions:  []*types.TransactionRecord{tx1 , tx2 , tx3} , UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber} , UnicitySeal: &types.UnicitySeal{}}}

	tx1Hash:= hex.EncodeToString(tx1.Hash(crypto.SHA256));

	// set
	txEx1, err := CreateTxExplorer(blockNumber, tx1)
	require.NoError(t, err)
	err = bs.Do().SetTxExplorerToBucket(txEx1)
	require.NoError(t, err)
	txEx2, err := CreateTxExplorer(blockNumber, tx2)
	require.NoError(t, err)
	err = bs.Do().SetTxExplorerToBucket(txEx2)
	require.NoError(t, err)
	txEx3, err := CreateTxExplorer(blockNumber, tx3)
	require.NoError(t, err)
	err = bs.Do().SetTxExplorerToBucket(txEx3)
	require.NoError(t, err)

	err = bs.Do().SetBlockExplorer(b)
	require.NoError(t, err)


	// get
	txs, err := bs.Do().GetBlockExplorerTxsByBlockNumber(blockNumber)
	require.NoError(t, err)
	require.NotNil(t, txs)
	require.EqualValues(t, len(txs) , 3)
	require.EqualValues(t, txs[0].BlockNumber , blockNumber)
	require.EqualValues(t, txs[0].Hash , tx1Hash)
	require.EqualValues(t, txs[0].Fee , tx1.ServerMetadata.ActualFee)

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

	bills, nextKey, err = bs.Do().GetBills(ownerPredicate1, true, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 2)

	// test get bill for unknown owner nok
	bills, nextKey, err = bs.Do().GetBills([]byte{1, 2, 3, 4}, true, nil, 100)
	require.NoError(t, err)
	require.Len(t, bills, 0)
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
	actualTxHistoryRecords, key, err := bs.Do().GetTxHistoryRecordsByKey(randomBytes, nil, 2)
	require.NoError(t, err)
	require.Len(t, actualTxHistoryRecords, 2)
	require.EqualValues(t, actualTxHistoryRecords[0].UnitID, []byte{max})
	require.NotNil(t, key)

	// verify tx history records are retrieved, all records
	var allTxHistoryRecords []*sdk.TxHistoryRecord
	key = nil
	for {
		actualTxHistoryRecords, key, err = bs.Do().GetTxHistoryRecordsByKey(randomBytes, key, 2)
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
	var bills []*Bill
	for i := byte(0); i < 10; i++ {
		b := newBillWithValueAndOwner(i, ownerPredicate)
		b.DCTargetUnitID = test.RandomBytes(32)
		b.DCTargetUnitBacklink = test.RandomBytes(32)
		bills = append(bills, b)

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
	dbFile := filepath.Join(t.TempDir(), BoltExplorerStoreFileName)
	store, err := newBoltBillStore(dbFile)
	require.NoError(t, err)
	return store
}

func getOwnerPredicate(pubkey string) []byte {
	pubKey, _ := hexutil.Decode(pubkey)
	return script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey))
}

func newBillWithValueAndOwner(val byte, ownerPredicate []byte) *Bill {
	return &Bill{
		Id:             newBillID(val),
		Value:          uint64(val),
		OwnerPredicate: ownerPredicate,
	}
}
