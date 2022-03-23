package wallet

import (
	"crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSwapIsTriggeredWhenDcSumIsReached(t *testing.T) {
	// create wallet with 2 normal bills
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)

	// when dc runs
	err := w.collectDust(false)
	require.NoError(t, err)

	// then metadata is updated
	dcNonce := calculateExpectedDcNonce(t, w)
	verifyBlockHeight(t, w, 0)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount, SwapTimeout: 0})

	// and two dc txs are broadcast
	require.Len(t, mockClient.txs, 2)
	for _, tx := range mockClient.txs {
		require.NotNil(t, parseDcTx(t, tx))
	}

	// when the block with dc txs is received
	block := &alphabill.Block{
		BlockNo:            1,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs,
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(&alphabill.GetBlocksResponse{Block: block})
	require.NoError(t, err)

	// then metadata is updated
	verifyBlockHeight(t, w, 1)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 0, DcTimeout: 0, SwapTimeout: 1 + swapTimeoutBlockCount})

	// and swap tx is broadcast
	require.Len(t, mockClient.txs, 3) // 2 dc + 1 swap
	tx := mockClient.txs[2]
	txSwap := parseSwapTx(t, tx)

	// and swap tx contains the exact same individual dc txs
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dustTransferTx := parseDcTx(t, mockClient.txs[i])
		dustTransferTxInSwap := parseDcTx(t, txSwap.DcTransfers[i])
		require.EqualValues(t, dustTransferTx.TargetBearer, dustTransferTxInSwap.TargetBearer)
		require.EqualValues(t, dustTransferTx.TargetValue, dustTransferTxInSwap.TargetValue)
		require.EqualValues(t, dustTransferTx.Backlink, dustTransferTxInSwap.Backlink)
		require.EqualValues(t, dustTransferTx.Nonce, dustTransferTxInSwap.Nonce)
	}

	// when further blocks are received
	for blockHeight := uint64(2); blockHeight <= dcTimeoutBlockCount; blockHeight++ {
		block = &alphabill.Block{
			BlockNo:            blockHeight,
			PrevBlockHash:      hash.Sum256([]byte{}),
			Transactions:       []*transaction.Transaction{},
			UnicityCertificate: []byte{},
		}
		err = w.processBlock(&alphabill.GetBlocksResponse{Block: block})
		require.NoError(t, err)
	}

	// then no more swap txs should be triggered
	require.Len(t, mockClient.txs, 3) // 2 dc + 1 swap

	// and only blockHeight is updated
	verifyBlockHeight(t, w, dcTimeoutBlockCount)
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 0, DcTimeout: 0, SwapTimeout: 1 + swapTimeoutBlockCount})

	// when swap tx block is received
	err = w.db.SetBlockHeight(swapTimeoutBlockCount)
	require.NoError(t, err)
	block = &alphabill.Block{
		BlockNo:            swapTimeoutBlockCount + 1,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs[2:3], // swap tx
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(&alphabill.GetBlocksResponse{Block: block})
	require.NoError(t, err)

	// then dc metadata is cleared
	verifyDcMetadataEmpty(t, w, dcNonce)
	verifyBlockHeight(t, w, swapTimeoutBlockCount+1)
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
	err := w.db.SetBlockHeight(dcTimeoutBlockCount - 1)
	require.NoError(t, err)
	block := &alphabill.Block{
		BlockNo:            dcTimeoutBlockCount,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(&alphabill.GetBlocksResponse{Block: block})
	require.NoError(t, err)

	// then swap should be broadcast
	require.Len(t, mockClient.txs, 1)

	// verify swap tx
	tx := mockClient.txs[0]
	swapTx := parseSwapTx(t, tx)
	require.EqualValues(t, nonce32[:], tx.UnitId)
	require.Len(t, swapTx.DcTransfers, 1)
	dcTx := parseDcTx(t, swapTx.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)
	require.EqualValues(t, 2, dcTx.TargetValue)

	// and metadata is updated
	verifyBlockHeight(t, w, dcTimeoutBlockCount)
	verifyDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: dcTimeoutBlockCount + swapTimeoutBlockCount})
	verifyBalance(t, w, 3)
}

func TestSwapIsTriggeredWhenSwapTimeoutIsReached(t *testing.T) {
	// wallet contains 1 dc bill and 1 normal bill
	w, mockClient := CreateTestWallet(t)
	addBill(t, w, 1)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addDcBill(t, w, nonce, 2, 10)
	setBlockHeight(t, w, swapTimeoutBlockCount-1)
	setDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: swapTimeoutBlockCount})

	// when swap timeout is reached
	block := &alphabill.Block{
		BlockNo:            swapTimeoutBlockCount,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err := w.processBlock(&alphabill.GetBlocksResponse{Block: block})
	require.NoError(t, err)

	// then swap tx is broadcast
	require.Len(t, mockClient.txs, 1)
	tx := mockClient.txs[0]
	swapTx := parseSwapTx(t, tx)
	require.EqualValues(t, nonce32[:], tx.UnitId)
	require.NotNil(t, swapTx)
	require.Len(t, swapTx.DcTransfers, 1)
	dcTx := parseDcTx(t, swapTx.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)

	// and metadata is updated
	verifyBlockHeight(t, w, swapTimeoutBlockCount)
	verifyDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: swapTimeoutBlockCount * 2})
	verifyBalance(t, w, 3)
}

func TestMetadataIsClearedWhenDcTimeoutIsReached(t *testing.T) {
	// create wallet with 2 normal bills with metadata as if dust txs were sent but not yet confirmed
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)
	dcNonce := calculateExpectedDcNonce(t, w)
	setBlockHeight(t, w, dcTimeoutBlockCount-1)
	setDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount, SwapTimeout: 0})

	// when dc timeout is reached
	block := &alphabill.Block{
		BlockNo:            dcTimeoutBlockCount,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err := w.processBlock(&alphabill.GetBlocksResponse{Block: block})
	require.NoError(t, err)

	// then no tx is broadcast
	require.Len(t, mockClient.txs, 0)

	// and metadata is cleared
	verifyDcMetadataEmpty(t, w, dcNonce)
	verifyBalance(t, w, 3)
}

func TestDcNonceHashIsCalculatedInCorrectBillOrder(t *testing.T) {
	bills := []*bill{
		{Id: uint256.NewInt(2)},
		{Id: uint256.NewInt(1)},
		{Id: uint256.NewInt(0)},
	}
	hasher := crypto.SHA256.New()
	for i := len(bills) - 1; i >= 0; i-- {
		hasher.Write(bills[i].getId())
	}
	expectedNonce := hasher.Sum(nil)

	nonce := calculateDcNonce(bills)
	require.EqualValues(t, expectedNonce, nonce)
}

func TestSwapTxValuesAreCalculatedInCorrectBillOrder(t *testing.T) {
	w, _ := CreateTestWallet(t)
	k, _ := w.db.GetAccountKey()

	dcBills := []*bill{
		{Id: uint256.NewInt(2), DcTx: createRandomDcTx()},
		{Id: uint256.NewInt(1), DcTx: createRandomDcTx()},
		{Id: uint256.NewInt(0), DcTx: createRandomDcTx()},
	}
	dcNonce := calculateDcNonce(dcBills)

	tx, err := createSwapTx(k, dcBills, dcNonce, 10)
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

func addBills(t *testing.T, w *Wallet) {
	addBill(t, w, 1)
	addBill(t, w, 2)
}

func addBill(t *testing.T, w *Wallet, value uint64) *bill {
	b1 := bill{
		Id:     uint256.NewInt(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
	err := w.db.SetBill(&b1)
	require.NoError(t, err)
	return &b1
}

func addDcBills(t *testing.T, w *Wallet, nonce *uint256.Int, timeout uint64) {
	addDcBill(t, w, nonce, 1, timeout)
	addDcBill(t, w, nonce, 2, timeout)
}

func addDcBill(t *testing.T, w *Wallet, nonce *uint256.Int, value uint64, timeout uint64) *bill {
	nonceB32 := nonce.Bytes32()
	b := bill{
		Id:     uint256.NewInt(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
	k, _ := w.db.GetAccountKey()

	tx, err := createDustTx(k, &b, nonceB32[:], timeout)
	require.NoError(t, err)

	b.IsDcBill = true
	b.DcTx = tx
	b.DcNonce = nonceB32[:]
	b.DcTimeout = timeout

	err = w.db.SetBill(&b)
	require.NoError(t, err)
	return &b
}

func verifyBlockHeight(t *testing.T, w *Wallet, blockHeight uint64) {
	actualBlockHeight, err := w.db.GetBlockHeight()
	require.NoError(t, err)
	require.Equal(t, blockHeight, actualBlockHeight)
}

func verifyDcMetadata(t *testing.T, w *Wallet, dcNonce []byte, m *dcMetadata) {
	require.NotEmpty(t, dcNonce)
	actualMetadata, err := w.db.GetDcMetadata(dcNonce)
	require.NoError(t, err)
	require.Equal(t, m.DcValueSum, actualMetadata.DcValueSum)
	require.Equal(t, m.DcTimeout, actualMetadata.DcTimeout)
	require.Equal(t, m.SwapTimeout, actualMetadata.SwapTimeout)
}

func verifyDcMetadataEmpty(t *testing.T, w *Wallet, dcNonce []byte) {
	require.NotEmpty(t, dcNonce)
	dcm, err := w.db.GetDcMetadata(dcNonce)
	require.NoError(t, err)
	require.Nil(t, dcm)
}

func setDcMetadata(t *testing.T, w *Wallet, dcNonce []byte, m *dcMetadata) {
	require.NotNil(t, dcNonce)
	err := w.db.SetDcMetadata(dcNonce, m)
	require.NoError(t, err)
}

func setBlockHeight(t *testing.T, w *Wallet, blockHeight uint64) {
	err := w.db.SetBlockHeight(blockHeight)
	require.NoError(t, err)
	err = w.db.SetMaxBlockHeight(blockHeight)
	require.NoError(t, err)
}

func setBlockHeightAndMaxBlockHeight(t *testing.T, w *Wallet, blockHeight uint64, maxBlockHeight uint64) {
	err := w.db.SetBlockHeight(blockHeight)
	require.NoError(t, err)
	err = w.db.SetMaxBlockHeight(maxBlockHeight)
	require.NoError(t, err)
}

func verifyBalance(t *testing.T, w *Wallet, balance uint64) {
	actualDcNonce, err := w.db.GetBalance()
	require.NoError(t, err)
	require.EqualValues(t, balance, actualDcNonce)
}

func parseBillTransferTx(t *testing.T, tx *transaction.Transaction) *transaction.BillTransfer {
	btTx := &transaction.BillTransfer{}
	err := tx.TransactionAttributes.UnmarshalTo(btTx)
	require.NoError(t, err)
	return btTx
}

func parseDcTx(t *testing.T, tx *transaction.Transaction) *transaction.TransferDC {
	dcTx := &transaction.TransferDC{}
	err := tx.TransactionAttributes.UnmarshalTo(dcTx)
	require.NoError(t, err)
	return dcTx
}

func parseSwapTx(t *testing.T, tx *transaction.Transaction) *transaction.Swap {
	txSwap := &transaction.Swap{}
	err := tx.TransactionAttributes.UnmarshalTo(txSwap)
	require.NoError(t, err)
	return txSwap
}

func calculateExpectedDcNonce(t *testing.T, w *Wallet) []byte {
	bills, err := w.db.GetBills()
	require.NoError(t, err)
	return calculateDcNonce(bills)
}
