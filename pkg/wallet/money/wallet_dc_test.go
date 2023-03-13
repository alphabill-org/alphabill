package money

import (
	"context"
	"crypto"
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSwapIsTriggeredWhenDcSumIsReached(t *testing.T) {
	// create wallet with 2 normal bills
	bills := []*Bill{addBill(1), addBill(2)}
	billsList := createBillListJsonResponse(bills)
	proofList := createBlockProofJsonResponse(t, bills, nil, 0, dcTimeoutBlockCount)

	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList})
	pubKey, _ := w.am.GetPublicKey(0)

	// when dc runs
	err := w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// and two dc txs are broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 2)
	for _, tx := range mockClient.GetRecordedTransactions() {
		require.NotNil(t, parseDcTx(t, tx))
	}

	// when the block with dc txs is received
	swapTimeout := uint64(swapTimeoutBlockCount + 1)
	mockClient.SetMaxBlockNumber(1)
	accKey, _ := w.am.GetAccountKey(0)
	nonce := calculateDcNonce(bills)
	dcBills := []*Bill{addDcBill(t, accKey, uint256.NewInt(0).SetBytes(nonce), 1, swapTimeoutBlockCount), addDcBill(t, accKey, uint256.NewInt(0).SetBytes(nonce), 2, swapTimeoutBlockCount)}
	billIds := [][]byte{util.Uint256ToBytes(dcBills[0].Id), util.Uint256ToBytes(dcBills[1].Id)}
	swapTx, _ := CreateSwapTx(accKey, dcBills, calculateDcNonce(dcBills), billIds, swapTimeout)
	_, _ = mockClient.SendTransaction(swapTx)

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
		mockClient.SetBlock(&block.Block{
			SystemIdentifier:   alphabillMoneySystemId,
			BlockNumber:        blockHeight,
			PreviousBlockHash:  hash.Sum256([]byte{}),
			Transactions:       []*txsystem.Transaction{},
			UnicityCertificate: &certificates.UnicityCertificate{},
		})
	}

	// then no more swap txs should be triggered
	require.Len(t, mockClient.GetRecordedTransactions(), 3) // 2 dc + 1 swap

	// and only blockHeight is updated
	verifyBlockHeight(t, w, dcTimeoutBlockCount)

	// when swap tx block is received
	mockClient.SetMaxBlockNumber(swapTimeout)
	require.NoError(t, err)
	verifyBlockHeight(t, w, swapTimeout)
	verifyBalance(t, w, 3, pubKey)
}

func TestSwapIsTriggeredWhenDcTimeoutIsReached(t *testing.T) {
	// create wallet with dc bill and non dc bill
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	w, _ := CreateTestWallet(t, nil)
	k, _ := w.am.GetAccountKey(0)
	bill := addBill(1)
	dc := addDcBill(t, k, nonce, 2, dcTimeoutBlockCount)
	billsList := createBillListJsonResponse([]*Bill{bill, dc})
	proofList := createBlockProofJsonResponse(t, []*Bill{bill, dc}, nil, 0, dcTimeoutBlockCount)

	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList})
	pubKey, _ := w.am.GetPublicKey(0)

	billIds := [][]byte{util.Uint256ToBytes(dc.Id)}
	swapTx, _ := CreateSwapTx(k, []*Bill{dc}, calculateDcNonce([]*Bill{dc}), billIds, 10)
	_, _ = mockClient.SendTransaction(swapTx)

	// when dcTimeout is reached
	mockClient.SetMaxBlockNumber(dcTimeoutBlockCount)

	// then swap should be broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)

	// verify swap tx
	tx := mockClient.GetRecordedTransactions()[0]
	swapOrder := parseSwapTx(t, tx)
	realNonce := calculateDcNonce([]*Bill{dc})
	require.EqualValues(t, realNonce[:], tx.UnitId)
	require.Len(t, swapOrder.DcTransfers, 1)
	dcTx := parseDcTx(t, swapOrder.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)
	require.EqualValues(t, 2, dcTx.TargetValue)

	verifyBlockHeight(t, w, dcTimeoutBlockCount)
	verifyTotalBalance(t, w, 3, pubKey)
}

func TestSwapIsTriggeredWhenSwapTimeoutIsReached(t *testing.T) {
	// wallet contains 1 dc bill and 1 normal bill
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	w, _ := CreateTestWallet(t, nil)
	k, _ := w.am.GetAccountKey(0)
	bill := addBill(1)
	dc := addDcBill(t, k, nonce, 2, swapTimeoutBlockCount)
	billsList := createBillListJsonResponse([]*Bill{bill, dc})
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: billsList})
	pubKey, _ := w.am.GetPublicKey(0)

	billIds := [][]byte{util.Uint256ToBytes(dc.Id)}
	swapTx, _ := CreateSwapTx(k, []*Bill{dc}, calculateDcNonce([]*Bill{dc}), billIds, swapTimeoutBlockCount)
	_, _ = mockClient.SendTransaction(swapTx)

	// when swapTimeout is reached
	mockClient.SetMaxBlockNumber(swapTimeoutBlockCount)

	// then swap tx is broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	tx := mockClient.GetRecordedTransactions()[0]
	swapOrder := parseSwapTx(t, tx)
	realNonce := calculateDcNonce([]*Bill{dc})
	require.EqualValues(t, realNonce[:], tx.UnitId)
	require.NotNil(t, swapOrder)
	require.Len(t, swapOrder.DcTransfers, 1)
	dcTx := parseDcTx(t, swapOrder.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)

	verifyBlockHeight(t, w, swapTimeoutBlockCount)
	verifyTotalBalance(t, w, 3, pubKey)
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
	w, _ := CreateTestWallet(t, nil)
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

	tx, err := CreateSwapTx(k, dcBills, dcNonce, dcBillIds, 10)
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

func TestSwapContainsUnconfirmedDustBillIds(t *testing.T) {
	// create wallet with three bills
	_ = log.InitStdoutLogger(log.INFO)
	b1 := addBill(1)
	b2 := addBill(2)
	b3 := addBill(3)

	billsList := createBillListJsonResponse([]*Bill{b1, b2, b3})
	proofList := createBlockProofJsonResponse(t, []*Bill{b1, b2, b3}, nil, 0, dcTimeoutBlockCount)
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: billsList, proofList: proofList})

	// when dc runs
	err := w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	verifyBlockHeight(t, w, 0)

	// and three dc txs are broadcast
	dcTxs := mockClient.GetRecordedTransactions()
	require.Len(t, dcTxs, 3)
	for _, tx := range dcTxs {
		require.NotNil(t, parseDcTx(t, tx))
	}

	// when 2 of 3 dc bills are confirmed before timeout
	mockClient.SetMaxBlockNumber(dcTimeoutBlockCount)

	accKey, _ := w.am.GetAccountKey(0)
	nonce := calculateDcNonce([]*Bill{b1, b2, b3})
	dcBills := []*Bill{addDcBill(t, accKey, uint256.NewInt(0).SetBytes(nonce), 1, 10), addDcBill(t, accKey, uint256.NewInt(0).SetBytes(nonce), 2, 10)}
	billIds := [][]byte{util.Uint256ToBytes(dcBills[0].Id), util.Uint256ToBytes(dcBills[1].Id), util.Uint256ToBytes(b3.Id)}
	swapTx, _ := CreateSwapTx(accKey, dcBills, calculateDcNonce(dcBills), billIds, 10)
	_, _ = mockClient.SendTransaction(swapTx)

	// then swap should be broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 4)

	// and swap should contain all bill ids
	tx := mockClient.GetRecordedTransactions()[3]
	swapOrder := parseSwapTx(t, tx)
	realNonce := calculateDcNonce(dcBills)
	require.EqualValues(t, realNonce, tx.UnitId)
	require.Len(t, swapOrder.BillIdentifiers, 3)
	require.Equal(t, b1.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[0]))
	require.Equal(t, b2.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[1]))
	require.Equal(t, b3.Id, uint256.NewInt(0).SetBytes(swapOrder.BillIdentifiers[2]))
	require.Len(t, swapOrder.DcTransfers, 2)
	require.Equal(t, dcTxs[0], swapOrder.DcTransfers[0])
	require.Equal(t, dcTxs[1], swapOrder.DcTransfers[1])

	swapTimeout := uint64(dcTimeoutBlockCount + swapTimeoutBlockCount)

	// when swap timeout is reached
	mockClient.SetMaxBlockNumber(swapTimeout)
	_, _ = mockClient.SendTransaction(swapTx)

	// then swap is broadcast with same bill ids
	require.Len(t, mockClient.GetRecordedTransactions(), 5)
	tx2 := mockClient.GetRecordedTransactions()[4]
	swapTx2 := parseSwapTx(t, tx2)
	require.Equal(t, swapOrder.BillIdentifiers, swapTx2.BillIdentifiers)
}

func addBill(value uint64) *Bill {
	b1 := Bill{
		Id:     uint256.NewInt(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
	return &b1
}

func addDcBill(t *testing.T, k *account.AccountKey, nonce *uint256.Int, value uint64, timeout uint64) *Bill {
	nonceB32 := nonce.Bytes32()
	b := Bill{
		Id:     uint256.NewInt(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}

	tx, err := CreateDustTx(k, &b, nonceB32[:], timeout)
	require.NoError(t, err)
	b.BlockProof = &BlockProof{Tx: tx}

	b.IsDcBill = true
	b.DcNonce = nonceB32[:]
	b.DcTimeout = timeout
	b.DcExpirationTimeout = dustBillDeletionTimeout

	require.NoError(t, err)
	return &b
}

func verifyBlockHeight(t *testing.T, w *Wallet, blockHeight uint64) {
	actualBlockHeight, _ := w.AlphabillClient.GetMaxBlockNumber()
	require.Equal(t, blockHeight, actualBlockHeight)
}

func verifyBalance(t *testing.T, w *Wallet, balance uint64, pubKey []byte) {
	actualBalance, err := w.restClient.GetBalance(pubKey, false)
	require.NoError(t, err)
	require.EqualValues(t, balance, actualBalance)
}

func verifyTotalBalance(t *testing.T, w *Wallet, balance uint64, pubKey []byte) {
	actualBalance, err := w.restClient.GetBalance(pubKey, true)
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
