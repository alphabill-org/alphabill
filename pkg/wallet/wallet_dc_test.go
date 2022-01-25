package wallet

import (
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSwapIsTriggeredWhenDcSumIsReached(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)

	// when dc runs
	err := w.collectDust()
	require.NoError(t, err)

	// then metadata is updated
	dcNonce := calculateExpectedDcNonce(t, w)
	verifyMetadata(t, w, metadata{blockHeight: 0, dcNonce: dcNonce, dcValueSum: 3, dcTimeout: dcTimeoutBlockCount, swapTimeout: 0})

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
	err = w.processBlock(block)
	require.NoError(t, err)

	// then metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: 1, dcNonce: dcNonce, dcValueSum: 0, dcTimeout: 0, swapTimeout: 1 + swapTimeoutBlockCount})

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
		err = w.processBlock(block)
		require.NoError(t, err)
	}

	// then no more swap txs should be triggered
	require.Len(t, mockClient.txs, 3) // 2 dc + 1 swap

	// and only blockHeight is updated
	verifyMetadata(t, w, metadata{blockHeight: dcTimeoutBlockCount, dcNonce: dcNonce, dcValueSum: 0, dcTimeout: 0, swapTimeout: 1 + swapTimeoutBlockCount})

	// when swap tx block is received
	err = w.db.SetBlockHeight(swapTimeoutBlockCount)
	require.NoError(t, err)
	block = &alphabill.Block{
		BlockNo:            swapTimeoutBlockCount + 1,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs[2:3], // swap tx
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then swap timeout is and dc nonce metadata is cleared
	verifyMetadata(t, w, metadata{blockHeight: 1 + swapTimeoutBlockCount, dcNonce: nil, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})
	verifyBalance(t, w, 3)
}

func TestSwapIsTriggeredWhenDcTimeoutIsReached(t *testing.T) {
	// create wallet with dc bill and non dc bill
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addBill(t, w, 1)
	addDcBill(t, w, nonce, 2, 10)
	setMetadata(t, w, metadata{blockHeight: 0, dcNonce: nonce32[:], dcValueSum: 3, dcTimeout: dcTimeoutBlockCount, swapTimeout: 0})

	// when dcTimeout is reached
	err := w.db.SetBlockHeight(dcTimeoutBlockCount - 1)
	require.NoError(t, err)
	block := &alphabill.Block{
		BlockNo:            dcTimeoutBlockCount,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
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
	verifyMetadata(t, w, metadata{
		blockHeight: dcTimeoutBlockCount,
		dcNonce:     nonce32[:],
		dcValueSum:  0,
		dcTimeout:   0,
		swapTimeout: dcTimeoutBlockCount + swapTimeoutBlockCount,
	})
	verifyBalance(t, w, 3)
}

func TestSwapIsTriggeredWhenSwapTimeoutIsReached(t *testing.T) {
	// wallet contains 1 managed dc bill and 1 normal bill
	w, mockClient := CreateTestWallet(t)
	addBill(t, w, 1)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addDcBill(t, w, nonce, 2, 10)
	setMetadata(t, w, metadata{blockHeight: swapTimeoutBlockCount - 1, dcNonce: nonce32[:], dcValueSum: 0, dcTimeout: 0, swapTimeout: swapTimeoutBlockCount})

	// when swap timeout is reached
	block := &alphabill.Block{
		BlockNo:            swapTimeoutBlockCount,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err := w.processBlock(block)
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
	verifyMetadata(t, w, metadata{
		blockHeight: swapTimeoutBlockCount,
		dcNonce:     nonce32[:],
		dcValueSum:  0,
		dcTimeout:   0,
		swapTimeout: swapTimeoutBlockCount * 2,
	})
	verifyBalance(t, w, 3)
}

func TestDcJobWithExistingUnmanagedDcBills(t *testing.T) {
	// wallet contains 2 dc bills with the same nonce that have timed out
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(1)
	nonce32 := nonce.Bytes32()
	addDcBills(t, w, nonce, 10)
	setMetadata(t, w, metadata{blockHeight: 100, dcNonce: nil, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs
	err := w.collectDust()
	require.NoError(t, err)

	// then swap tx is broadcast
	require.Len(t, mockClient.txs, 1)
	tx := mockClient.txs[0]
	txSwap := parseSwapTx(t, tx)

	// and verify each dc tx id = nonce = swap.id
	require.Len(t, txSwap.DcTransfers, 2)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTx(t, txSwap.DcTransfers[i])
		require.EqualValues(t, nonce32[:], dcTx.Nonce)
		require.EqualValues(t, nonce32[:], tx.UnitId)
	}

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: 100, dcNonce: nil, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})
}

func TestDcJobWithExistingUnmanagedDcAndNonDcBills(t *testing.T) {
	// wallet contains timed out dc bill and normal bill
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addBill(t, w, 1)
	addDcBill(t, w, nonce, 2, 10)
	setMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs
	err := w.collectDust()
	require.NoError(t, err)

	// then swap tx is sent with the existing dc bill
	require.Len(t, mockClient.txs, 1)
	tx := mockClient.txs[0]
	txSwap := parseSwapTx(t, tx)

	// and verify nonce = swap.id = dc tx id
	require.Len(t, txSwap.DcTransfers, 1)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTx(t, txSwap.DcTransfers[i])
		require.EqualValues(t, nonce32[:], dcTx.Nonce)
		require.EqualValues(t, nonce32[:], tx.UnitId)
	}

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})
}

func TestDcJobWithExistingNonDcBills(t *testing.T) {
	// wallet contains 2 non dc bills
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)
	setMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs
	err := w.collectDust()
	require.NoError(t, err)

	// then dust txs are broadcast
	require.Len(t, mockClient.txs, 2)

	// and nonces are equal
	dcTx0 := parseDcTx(t, mockClient.txs[0])
	dcTx1 := parseDcTx(t, mockClient.txs[1])
	require.EqualValues(t, dcTx0.Nonce, dcTx1.Nonce)

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: 100, dcNonce: dcTx0.Nonce, dcValueSum: 3, dcTimeout: 100 + dcTimeoutBlockCount, swapTimeout: 0})
}

func TestDcJobDoesNotSendSwapIfDcBillTimeoutHasNotBeenReached(t *testing.T) {
	// wallet contains 2 dc bills that have not yet timed out
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(1)
	addDcBills(t, w, nonce, 10)
	setMetadata(t, w, metadata{blockHeight: 5, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs
	err := w.collectDust()
	require.NoError(t, err)

	// then swap must not be broadcast
	require.Empty(t, mockClient.txs, 0)
}

func TestDcJobSendsMultipleSwapsIfDcBillTimeoutHasBeenReached(t *testing.T) {
	// wallet contains 2 dc bills that both have timed out
	w, mockClient := CreateTestWallet(t)
	nonce1 := uint256.NewInt(1)
	nonce2 := uint256.NewInt(2)
	addDcBill(t, w, nonce1, 1, 10)
	addDcBill(t, w, nonce2, 2, 10)
	setMetadata(t, w, metadata{blockHeight: 10, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs
	err := w.collectDust()
	require.NoError(t, err)

	// then 2 swap txs must be broadcast
	require.Len(t, mockClient.txs, 2)
	for _, tx := range mockClient.txs {
		require.NotNil(t, parseSwapTx(t, tx))
	}
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
	tx, err := w.createDustTx(&b, nonceB32[:], timeout)
	require.NoError(t, err)

	b.IsDcBill = true
	b.DcTx = tx
	b.DcNonce = nonceB32[:]
	b.DcTimeout = timeout

	err = w.db.SetBill(&b)
	require.NoError(t, err)
	return &b
}

type metadata struct {
	blockHeight uint64
	dcNonce     []byte
	dcValueSum  uint64
	dcTimeout   uint64
	swapTimeout uint64
}

func verifyMetadata(t *testing.T, w *Wallet, m metadata) {
	actualBlockHeight, err := w.db.GetBlockHeight()
	require.NoError(t, err)
	require.Equal(t, m.blockHeight, actualBlockHeight)

	actualDcNonce, err := w.db.GetDcNonce()
	require.NoError(t, err)
	require.Equal(t, m.dcNonce, actualDcNonce)

	actualDcValueSum, err := w.db.GetDcValueSum()
	require.NoError(t, err)
	require.Equal(t, m.dcValueSum, actualDcValueSum)

	actualDcTimeout, err := w.db.GetDcTimeout()
	require.NoError(t, err)
	require.Equal(t, m.dcTimeout, actualDcTimeout)

	actualSwapTimeout, err := w.db.GetSwapTimeout()
	require.NoError(t, err)
	require.Equal(t, m.swapTimeout, actualSwapTimeout)
}

func setMetadata(t *testing.T, w *Wallet, m metadata) {
	err := w.db.SetBlockHeight(m.blockHeight)
	require.NoError(t, err)

	err = w.db.SetDcNonce(m.dcNonce)
	require.NoError(t, err)

	err = w.db.SetDcValueSum(m.dcValueSum)
	require.NoError(t, err)

	err = w.db.SetDcTimeout(m.dcTimeout)
	require.NoError(t, err)

	err = w.db.SetSwapTimeout(m.swapTimeout)
	require.NoError(t, err)
}

func verifyBalance(t *testing.T, w *Wallet, balance uint64) {
	actualDcNonce, err := w.db.GetBalance()
	require.NoError(t, err)
	require.EqualValues(t, balance, actualDcNonce)
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
