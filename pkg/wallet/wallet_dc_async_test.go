package wallet

import (
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDcJobWithExistingDcBills(t *testing.T) {
	// wallet contains 2 dc bills with the same nonce that have timed out
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(1)
	nonce32 := nonce.Bytes32()
	addDcBills(t, w, nonce, 10)
	setBlockHeight(t, w, 100)
	mockClient.maxBlockNo = 100

	// when dust collector runs
	err := w.collectDust(false)
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
	verifyDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: 100 + swapTimeoutBlockCount})
}

func TestDcJobWithExistingDcAndNonDcBills(t *testing.T) {
	// wallet contains timed out dc bill and normal bill
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(2)
	nonce32 := nonce.Bytes32()
	addBill(t, w, 1)
	addDcBill(t, w, nonce, 2, 10)
	setBlockHeight(t, w, 100)
	mockClient.maxBlockNo = 100

	// when dust collector runs
	err := w.collectDust(false)
	require.NoError(t, err)

	// then swap tx is sent for the timed out dc bill
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
	verifyDcMetadata(t, w, nonce32[:], &dcMetadata{SwapTimeout: 100 + swapTimeoutBlockCount})
}

func TestDcJobWithExistingNonDcBills(t *testing.T) {
	// wallet contains 2 non dc bills
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)
	setBlockHeight(t, w, 100)
	mockClient.maxBlockNo = 100

	// when dust collector runs
	err := w.collectDust(false)
	require.NoError(t, err)

	// then dust txs are broadcast
	require.Len(t, mockClient.txs, 2)

	// and nonces are equal
	dcTx0 := parseDcTx(t, mockClient.txs[0])
	dcTx1 := parseDcTx(t, mockClient.txs[1])
	require.EqualValues(t, dcTx0.Nonce, dcTx1.Nonce)

	// and metadata is updated
	verifyDcMetadata(t, w, dcTx0.Nonce, &dcMetadata{DcValueSum: 3, DcTimeout: 100 + dcTimeoutBlockCount})
}

func TestDcJobDoesNotSendSwapIfDcBillTimeoutHasNotBeenReached(t *testing.T) {
	// wallet contains 2 dc bills that have not yet timed out
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(1)
	addDcBills(t, w, nonce, 10)
	setBlockHeight(t, w, 5)

	// when dust collector runs
	err := w.collectDust(false)
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
	setBlockHeight(t, w, 10)
	mockClient.maxBlockNo = 10

	// when dust collector runs
	err := w.collectDust(false)
	require.NoError(t, err)

	// then 2 swap txs must be broadcast
	require.Len(t, mockClient.txs, 2)
	for _, tx := range mockClient.txs {
		require.NotNil(t, parseSwapTx(t, tx))
	}

	// and 2 dc metadata entries are saved
	n1b32 := nonce1.Bytes32()
	n2b32 := nonce2.Bytes32()
	verifyDcMetadata(t, w, n1b32[:], &dcMetadata{SwapTimeout: 10 + swapTimeoutBlockCount})
	verifyDcMetadata(t, w, n2b32[:], &dcMetadata{SwapTimeout: 10 + swapTimeoutBlockCount})
}

func TestConcurrentDcJobCannotBeStarted(t *testing.T) {
	// wallet contains 2 normal bills and metadata that dc process was started
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)
	dcNonce := calculateExpectedDcNonce(t, w)
	setDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount})

	// when dust collector runs
	err := w.collectDust(false)
	require.NoError(t, err)

	// then no tx must not be broadcast
	require.Len(t, mockClient.txs, 0)

	// and metadata is the same
	verifyDcMetadata(t, w, dcNonce, &dcMetadata{DcValueSum: 3, DcTimeout: dcTimeoutBlockCount})
}
