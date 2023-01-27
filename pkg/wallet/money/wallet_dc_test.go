package money

import (
	"context"
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestSwapIsTriggeredWhenDcSumIsReached(t *testing.T) {
	// create wallet with 2 normal bills
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)

	// when dc runs
	err := w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then metadata is updated
	dcNonce := calculateExpectedDcNonce(t, w)
	verifyBlockHeight(t, w, 0)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount, SwapTimeout: 0})

	// and two dc txs are broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 2)
	for _, tx := range mockClient.GetRecordedTransactions() {
		require.NotNil(t, parseDcTx(t, tx))
	}

	// when the block with dc txs is received
	swapTimeout := uint64(swapTimeoutBlockCount + 1)
	mockClient.SetMaxBlockNumber(1)
	b := &block.Block{
		SystemIdentifier:   alphabillMoneySystemId,
		BlockNumber:        1,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       mockClient.GetRecordedTransactions(),
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	err = w.ProcessBlock(b)
	require.NoError(t, err)

	// then metadata is updated
	verifyBlockHeight(t, w, 1)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 0, DcTimeout: 0, SwapTimeout: swapTimeout})

	// and swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 3) // 2 dc + 1 swap
	tx := mockClient.GetRecordedTransactions()[2]
	txSwap := parseSwapTx(t, tx)

	// and swap tx contains the exact same individual dc txs
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dustTransferTx := parseDcTx(t, mockClient.GetRecordedTransactions()[i])
		dustTransferTxInSwap := parseDcTx(t, txSwap.DcTransfers[i])
		require.EqualValues(t, dustTransferTx.TargetBearer, dustTransferTxInSwap.TargetBearer)
		require.EqualValues(t, dustTransferTx.TargetValue, dustTransferTxInSwap.TargetValue)
		require.EqualValues(t, dustTransferTx.Backlink, dustTransferTxInSwap.Backlink)
		require.EqualValues(t, dustTransferTx.Nonce, dustTransferTxInSwap.Nonce)
	}

	// when further blocks are received
	mockClient.SetMaxBlockNumber(dcTimeoutBlockCount)
	for blockHeight := uint64(2); blockHeight <= dcTimeoutBlockCount; blockHeight++ {
		b = &block.Block{
			SystemIdentifier:   alphabillMoneySystemId,
			BlockNumber:        blockHeight,
			PreviousBlockHash:  hash.Sum256([]byte{}),
			Transactions:       []*txsystem.Transaction{},
			UnicityCertificate: &certificates.UnicityCertificate{},
		}
		err = w.ProcessBlock(b)
		require.NoError(t, err)
	}

	// then no more swap txs should be triggered
	require.Len(t, mockClient.GetRecordedTransactions(), 3) // 2 dc + 1 swap

	// and only blockHeight is updated
	verifyBlockHeight(t, w, dcTimeoutBlockCount)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 0, DcTimeout: 0, SwapTimeout: swapTimeout})

	// when swap tx block is received
	mockClient.SetMaxBlockNumber(swapTimeout)
	err = w.db.Do().SetBlockNumber(swapTimeoutBlockCount)
	require.NoError(t, err)
	b = &block.Block{
		SystemIdentifier:   alphabillMoneySystemId,
		BlockNumber:        swapTimeout,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       mockClient.GetRecordedTransactions()[2:3], // swap tx
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	err = w.ProcessBlock(b)
	require.NoError(t, err)

	// then dc metadata is cleared
	verifyDcMetadataEmpty(t, w, dcNonce)
	verifyBlockHeight(t, w, swapTimeout)
	verifyBalance(t, w, 3)
}

func TestSwapIsTriggeredWhenDcTimeoutIsReached(t *testing.T) {
	// create wallet with dc bill and non dc bill
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addBill(t, w, 1)
	addDcBill(t, w, nonce, 2, 10)
	setDcMetadata(t, w, nonce32[:], &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount, SwapTimeout: 0})

	// when dcTimeout is reached
	mockClient.SetMaxBlockNumber(dcTimeoutBlockCount)
	err := w.db.Do().SetBlockNumber(dcTimeoutBlockCount - 1)
	require.NoError(t, err)
	b := &block.Block{
		SystemIdentifier:   alphabillMoneySystemId,
		BlockNumber:        dcTimeoutBlockCount,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	err = w.ProcessBlock(b)
	require.NoError(t, err)

	// then swap should be broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)

	// verify swap tx
	tx := mockClient.GetRecordedTransactions()[0]
	swapTx := parseSwapTx(t, tx)
	require.EqualValues(t, nonce32[:], tx.UnitId)
	require.Len(t, swapTx.DcTransfers, 1)
	dcTx := parseDcTx(t, swapTx.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)
	require.EqualValues(t, 2, dcTx.TargetValue)

	// and metadata is updated
	verifyBlockHeight(t, w, dcTimeoutBlockCount)
	verifyDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: dcTimeoutBlockCount + swapTimeoutBlockCount})
	verifyBalance(t, w, 1)
	verifyTotalBalance(t, w, 3)
}

func TestSwapIsTriggeredWhenSwapTimeoutIsReached(t *testing.T) {
	// wallet contains 1 dc bill and 1 normal bill
	w, mockClient := CreateTestWallet(t)
	addBill(t, w, 1)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addDcBill(t, w, nonce, 2, 10)
	setBlockHeight(t, w, swapTimeoutBlockCount-1)
	mockClient.SetMaxBlockNumber(swapTimeoutBlockCount)
	setDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: swapTimeoutBlockCount})

	// when swap timeout is reached
	b := &block.Block{
		SystemIdentifier:   alphabillMoneySystemId,
		BlockNumber:        swapTimeoutBlockCount,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	err := w.ProcessBlock(b)
	require.NoError(t, err)

	// then swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	tx := mockClient.GetRecordedTransactions()[0]
	swapTx := parseSwapTx(t, tx)
	require.EqualValues(t, nonce32[:], tx.UnitId)
	require.NotNil(t, swapTx)
	require.Len(t, swapTx.DcTransfers, 1)
	dcTx := parseDcTx(t, swapTx.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)

	// and metadata is updated
	verifyBlockHeight(t, w, swapTimeoutBlockCount)
	verifyDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: swapTimeoutBlockCount * 2})
	verifyBalance(t, w, 1)
	verifyTotalBalance(t, w, 3)
}

func TestMetadataIsClearedWhenDcTimeoutIsReached(t *testing.T) {
	// create wallet with 2 normal bills with metadata as if dust txs were sent but not yet confirmed
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)
	dcNonce := calculateExpectedDcNonce(t, w)
	setBlockHeight(t, w, dcTimeoutBlockCount-1)
	setDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount, SwapTimeout: 0})

	// when dc timeout is reached
	b := &block.Block{
		SystemIdentifier:   alphabillMoneySystemId,
		BlockNumber:        dcTimeoutBlockCount,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	err := w.ProcessBlock(b)
	require.NoError(t, err)

	// then no tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 0)

	// and metadata is cleared
	verifyDcMetadataEmpty(t, w, dcNonce)
	verifyBalance(t, w, 3)
}

func TestDcNonceHashIsCalculatedInCorrectBillOrder(t *testing.T) {
	bills := []*Bill{
		{Id: uint256.NewInt(2)},
		{Id: uint256.NewInt(1)},
		{Id: uint256.NewInt(0)},
	}
	hasher := crypto.SHA256.New()
	for i := len(bills) - 1; i >= 0; i-- {
		hasher.Write(bills[i].GetID())
	}
	expectedNonce := hasher.Sum(nil)

	nonce := calculateDcNonce(bills)
	require.EqualValues(t, expectedNonce, nonce)
}

func TestSwapTxValuesAreCalculatedInCorrectBillOrder(t *testing.T) {
	w, _ := CreateTestWallet(t)
	k, _ := w.am.GetAccountKey(0)

	dcBills := []*Bill{
		{Id: uint256.NewInt(2), BlockProof: &BlockProof{Tx: moneytesttx.CreateRandomDcTx()}},
		{Id: uint256.NewInt(1), BlockProof: &BlockProof{Tx: moneytesttx.CreateRandomDcTx()}},
		{Id: uint256.NewInt(0), BlockProof: &BlockProof{Tx: moneytesttx.CreateRandomDcTx()}},
	}
	dcNonce := calculateDcNonce(dcBills)
	var dcBillIds [][]byte
	for _, dcBill := range dcBills {
		dcBillIds = append(dcBillIds, dcBill.GetID())
	}

	tx, err := createSwapTx(k, dcBills, dcNonce, dcBillIds, 10)
	require.NoError(t, err)
	swapTx := parseSwapTx(t, tx)

	// verify bill ids in swap tx are in correct order (equal hash values)
	hasher := crypto.SHA256.New()
	for _, billId := range swapTx.BillIdentifiers {
		hasher.Write(billId)
	}
	actualDcNonce := hasher.Sum(nil)
	require.EqualValues(t, dcNonce, actualDcNonce)
}

func TestExpiredDcBillsGetDeleted(t *testing.T) {
	w, _ := CreateTestWallet(t)
	b1 := &Bill{Id: uint256.NewInt(0), IsDcBill: false}
	b2 := &Bill{Id: uint256.NewInt(1), IsDcBill: true, DcExpirationTimeout: 10}
	b3 := &Bill{Id: uint256.NewInt(2), IsDcBill: true, DcExpirationTimeout: 20}
	_ = w.db.Do().SetBill(0, b1)
	_ = w.db.Do().SetBill(0, b2)
	_ = w.db.Do().SetBill(0, b3)
	blockHeight := uint64(15)
	_ = w.db.Do().SetBlockNumber(blockHeight)

	// verify initial bills
	require.False(t, b1.isExpired(blockHeight))
	require.True(t, b2.isExpired(blockHeight))
	require.False(t, b3.isExpired(blockHeight))

	// receiving a block should delete expired bills
	err := w.ProcessBlock(&block.Block{
		SystemIdentifier: alphabillMoneySystemId,
		BlockNumber:      blockHeight + 1,
		Transactions:     []*txsystem.Transaction{},
	})
	require.NoError(t, err)

	// verify that one expired bill gets removed and remaining bills are not expired
	bills, _ := w.db.Do().GetBills(0)
	require.Len(t, bills, 2)
	for _, b := range bills {
		require.False(t, b.isExpired(blockHeight))
	}
}

func TestSwapContainsUnconfirmedDustBillIds(t *testing.T) {
	// create wallet with three bills
	_ = log.InitStdoutLogger(log.INFO)
	w, mockClient := CreateTestWallet(t)
	b1 := addBill(t, w, 1)
	b2 := addBill(t, w, 2)
	b3 := addBill(t, w, 3)

	// when dc runs
	err := w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then metadata is updated
	dcNonce := calculateExpectedDcNonce(t, w)
	verifyBlockHeight(t, w, 0)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 6, DcTimeout: dcTimeoutBlockCount, SwapTimeout: 0})

	// and three dc txs are broadcast
	dcTxs := mockClient.GetRecordedTransactions()
	require.Len(t, dcTxs, 3)
	for _, tx := range dcTxs {
		require.NotNil(t, parseDcTx(t, tx))
	}

	// when 2 of 3 dc bills are confirmed before timeout
	mockClient.SetMaxBlockNumber(dcTimeoutBlockCount)
	_ = w.db.Do().SetBlockNumber(dcTimeoutBlockCount - 1)
	b := &block.Block{
		SystemIdentifier:   alphabillMoneySystemId,
		BlockNumber:        dcTimeoutBlockCount,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       dcTxs[0:2],
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	err = w.ProcessBlock(b)
	require.NoError(t, err)

	// then swap should be broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 4)

	// and swap should contain all bill ids
	tx := mockClient.GetRecordedTransactions()[3]
	swapTx := parseSwapTx(t, tx)
	require.EqualValues(t, dcNonce, tx.UnitId)
	require.Len(t, swapTx.BillIdentifiers, 3)
	require.Equal(t, b1.Id, uint256.NewInt(0).SetBytes(swapTx.BillIdentifiers[0]))
	require.Equal(t, b2.Id, uint256.NewInt(0).SetBytes(swapTx.BillIdentifiers[1]))
	require.Equal(t, b3.Id, uint256.NewInt(0).SetBytes(swapTx.BillIdentifiers[2]))
	require.Len(t, swapTx.DcTransfers, 2)
	require.Equal(t, dcTxs[0], swapTx.DcTransfers[0])
	require.Equal(t, dcTxs[1], swapTx.DcTransfers[1])

	// and metadata is updated
	swapTimeout := uint64(dcTimeoutBlockCount + swapTimeoutBlockCount)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{SwapTimeout: swapTimeout})

	// when swap timeout is reached
	mockClient.SetMaxBlockNumber(swapTimeout)
	_ = w.db.Do().SetBlockNumber(swapTimeout - 1)
	b = &block.Block{
		SystemIdentifier:   alphabillMoneySystemId,
		BlockNumber:        swapTimeout,
		PreviousBlockHash:  hash.Sum256([]byte{}),
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}
	err = w.ProcessBlock(b)
	require.NoError(t, err)

	// then swap is broadcast with same bill ids
	require.Len(t, mockClient.GetRecordedTransactions(), 5)
	tx2 := mockClient.GetRecordedTransactions()[4]
	swapTx2 := parseSwapTx(t, tx2)
	require.Equal(t, swapTx.BillIdentifiers, swapTx2.BillIdentifiers)
}

func addBills(t *testing.T, w *Wallet) {
	addBill(t, w, 1)
	addBill(t, w, 2)
}

func addBill(t *testing.T, w *Wallet, value uint64) *Bill {
	b1 := Bill{
		Id:     uint256.NewInt(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
	err := w.db.Do().SetBill(0, &b1)
	require.NoError(t, err)
	return &b1
}

func addDcBills(t *testing.T, w *Wallet, nonce *uint256.Int, timeout uint64) {
	addDcBill(t, w, nonce, 1, timeout)
	addDcBill(t, w, nonce, 2, timeout)
}

func addDcBill(t *testing.T, w *Wallet, nonce *uint256.Int, value uint64, timeout uint64) *Bill {
	nonceB32 := nonce.Bytes32()
	b := Bill{
		Id:     uint256.NewInt(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
	k, _ := w.am.GetAccountKey(0)

	tx, err := createDustTx(k, &b, nonceB32[:], timeout)
	require.NoError(t, err)
	b.BlockProof = &BlockProof{Tx: tx}

	b.IsDcBill = true
	b.DcNonce = nonceB32[:]
	b.DcTimeout = timeout
	b.DcExpirationTimeout = dustBillDeletionTimeout

	err = w.db.Do().SetBill(0, &b)
	require.NoError(t, err)
	return &b
}

func verifyBlockHeight(t *testing.T, w *Wallet, blockHeight uint64) {
	actualBlockHeight, _ := w.db.Do().GetBlockNumber()
	require.Equal(t, blockHeight, actualBlockHeight)
}

func verifyDcMetadata(t *testing.T, w *Wallet, dcNonce []byte, m *dcMetadata) {
	require.NotEmpty(t, dcNonce)
	actualMetadata, err := w.db.Do().GetDcMetadata(0, dcNonce)
	require.NoError(t, err)
	require.Equal(t, m.DcValueSum, actualMetadata.DcValueSum)
	require.Equal(t, m.DcTimeout, actualMetadata.DcTimeout)
	require.Equal(t, m.SwapTimeout, actualMetadata.SwapTimeout)
}

func verifyDcMetadataEmpty(t *testing.T, w *Wallet, dcNonce []byte) {
	require.NotEmpty(t, dcNonce)
	dcm, err := w.db.Do().GetDcMetadata(0, dcNonce)
	require.NoError(t, err)
	require.Nil(t, dcm)
}

func setDcMetadata(t *testing.T, w *Wallet, dcNonce []byte, m *dcMetadata) {
	require.NotNil(t, dcNonce)
	err := w.db.Do().SetDcMetadata(0, dcNonce, m)
	require.NoError(t, err)
}

func setBlockHeight(t *testing.T, w *Wallet, blockHeight uint64) {
	err := w.db.Do().SetBlockNumber(blockHeight)
	require.NoError(t, err)
}

func verifyBalance(t *testing.T, w *Wallet, balance uint64) {
	actualBalance, err := w.db.Do().GetBalance(GetBalanceCmd{})
	require.NoError(t, err)
	require.EqualValues(t, balance, actualBalance)
}

func verifyTotalBalance(t *testing.T, w *Wallet, balance uint64) {
	actualBalance, err := w.db.Do().GetBalance(GetBalanceCmd{CountDCBills: true})
	require.NoError(t, err)
	require.EqualValues(t, balance, actualBalance)
}

func parseBillTransferTx(t *testing.T, tx *txsystem.Transaction) *billtx.TransferOrder {
	btTx := &billtx.TransferOrder{}
	err := tx.TransactionAttributes.UnmarshalTo(btTx)
	require.NoError(t, err)
	return btTx
}

func parseDcTx(t *testing.T, tx *txsystem.Transaction) *billtx.TransferDCOrder {
	dcTx := &billtx.TransferDCOrder{}
	err := tx.TransactionAttributes.UnmarshalTo(dcTx)
	require.NoError(t, err)
	return dcTx
}

func parseSwapTx(t *testing.T, tx *txsystem.Transaction) *billtx.SwapOrder {
	txSwap := &billtx.SwapOrder{}
	err := tx.TransactionAttributes.UnmarshalTo(txSwap)
	require.NoError(t, err)
	return txSwap
}

func calculateExpectedDcNonce(t *testing.T, w *Wallet) []byte {
	bills, err := w.db.Do().GetBills(0)
	require.NoError(t, err)
	return calculateDcNonce(bills)
}
