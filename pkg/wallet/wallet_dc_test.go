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
	verifyMetadata(t, w, metadata{blockHeight: 0, dcValueSum: 3, dcTimeout: dcTimeoutBlockCount, swapTimeout: 0})

	// and two dc txs are broadcast
	require.Len(t, mockClient.txs, 2)

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
	verifyMetadata(t, w, metadata{blockHeight: 1, dcValueSum: 0, dcTimeout: 0, swapTimeout: 1 + swapTimeoutBlockCount})

	// and swap tx is broadcast
	require.Len(t, mockClient.txs, 3) // 2 dc + 1 swap
	txSwapPb := mockClient.txs[2]
	txSwap := &transaction.Swap{}
	err = txSwapPb.TransactionAttributes.UnmarshalTo(txSwap)
	require.NoError(t, err)

	// and swap tx contains the exact same individual dc txs
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		mockClientTx := mockClient.txs[i]
		dustTransferTx := &transaction.TransferDC{}
		err = mockClientTx.TransactionAttributes.UnmarshalTo(dustTransferTx)
		require.NoError(t, err)

		dustTransferSwapTx := txSwap.DcTransfers[i]
		dustTransferTxInSwap := &transaction.TransferDC{}
		err = dustTransferSwapTx.TransactionAttributes.UnmarshalTo(dustTransferTxInSwap)
		require.NoError(t, err)
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
	verifyMetadata(t, w, metadata{blockHeight: dcTimeoutBlockCount, dcValueSum: 0, dcTimeout: 0, swapTimeout: 1 + swapTimeoutBlockCount})

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

	// then swap timeout is cleared
	verifyMetadata(t, w, metadata{blockHeight: 1 + swapTimeoutBlockCount, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})
	verifyBalance(t, w, 3)
}

func TestSwapIsTriggeredWhenDcTimeoutIsReached(t *testing.T) {
	// create wallet with dc bill and non dc bill
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(0)
	nonce32 := nonce.Bytes32()
	addBill(t, w, 1)
	addDcBill(t, w, nonce, 2)
	setMetadata(t, w, metadata{blockHeight: 0, dcValueSum: 3, dcTimeout: dcTimeoutBlockCount, swapTimeout: 0})

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
	tx, swapTx := parseSwapTx(t, mockClient)
	require.EqualValues(t, nonce32[:], tx.UnitId)
	require.Len(t, swapTx.DcTransfers, 1)
	dcTx := parseDcTx(t, swapTx.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)
	require.EqualValues(t, 2, dcTx.TargetValue)

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: dcTimeoutBlockCount, dcValueSum: 0, dcTimeout: 0, swapTimeout: dcTimeoutBlockCount + swapTimeoutBlockCount})
	verifyBalance(t, w, 3)
}

func TestSwapIsTriggeredWhenSwapTimeoutIsReached(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	addBill(t, w, 1)
	nonce := uint256.NewInt(0)
	nonce32 := nonce.Bytes32()
	addDcBill(t, w, nonce, 2)
	setMetadata(t, w, metadata{blockHeight: swapTimeoutBlockCount - 1, dcValueSum: 0, dcTimeout: 0, swapTimeout: swapTimeoutBlockCount})

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
	tx, swapTx := parseSwapTx(t, mockClient)
	require.EqualValues(t, nonce32[:], tx.UnitId)
	require.NotNil(t, swapTx)
	require.Len(t, swapTx.DcTransfers, 1)
	dcTx := parseDcTx(t, swapTx.DcTransfers[0])
	require.EqualValues(t, nonce32[:], dcTx.Nonce)

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: swapTimeoutBlockCount, dcValueSum: 0, dcTimeout: 0, swapTimeout: swapTimeoutBlockCount * 2})
	verifyBalance(t, w, 3)
}

func TestDcProcessWithExistingDcBills(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(0)
	nonce32 := nonce.Bytes32()
	addDcBills(t, w, nonce)
	setMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs and only confirmed dc bills exist
	err := w.collectDust()
	require.NoError(t, err)

	// then swap tx is broadcast
	require.Len(t, mockClient.txs, 1)
	txSwapPb, txSwap := parseSwapTx(t, mockClient)

	// and verify each dc tx id = nonce = swap.id
	require.Len(t, txSwap.DcTransfers, 2)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTx(t, txSwap.DcTransfers[i])
		require.EqualValues(t, nonce32[:], dcTx.Nonce)
		require.EqualValues(t, nonce32[:], txSwapPb.UnitId)
	}

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 0, dcTimeout: 0, swapTimeout: 100 + swapTimeoutBlockCount})
}

func TestDcProcessWithExistingDcAndNonDcBills(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	nonce := uint256.NewInt(0)
	nonce32 := nonce.Bytes32()
	addBill(t, w, 1)
	addDcBill(t, w, nonce, 2)
	setMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 0, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs and both dc and non-dc bills exists
	err := w.collectDust()
	require.NoError(t, err)

	// then swap tx is sent with the existing dc bill
	require.Len(t, mockClient.txs, 1)
	txSwapPb, txSwap := parseSwapTx(t, mockClient)

	// and verify nonce = swap.id = dc tx id
	require.Len(t, txSwap.DcTransfers, 1)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTx(t, txSwap.DcTransfers[i])
		require.EqualValues(t, nonce32[:], dcTx.Nonce)
		require.EqualValues(t, nonce32[:], txSwapPb.UnitId)
	}

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 0, dcTimeout: 0, swapTimeout: 100 + swapTimeoutBlockCount})
}

func TestDcProcessWithExistingNonDcBills(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	addBills(t, w)
	setMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 3, dcTimeout: 0, swapTimeout: 0})

	// when dust collector runs
	err := w.collectDust()
	require.NoError(t, err)

	// then dust txs are broadcast
	require.Len(t, mockClient.txs, 2)

	// and nonces are equal
	nonce := []byte{
		214, 186, 147, 41, 248, 147, 44, 18, 25, 43, 55, 132, 159, 119, 33, 4,
		210, 0, 72, 247, 100, 52, 163, 41, 5, 18, 217, 216, 20, 228, 17, 111,
	}
	for i := 0; i < len(mockClient.txs); i++ {
		dcTx := parseDcTx(t, mockClient.txs[i])
		require.EqualValues(t, nonce, dcTx.Nonce)
	}

	// and metadata is updated
	verifyMetadata(t, w, metadata{blockHeight: 100, dcValueSum: 3, dcTimeout: 100 + dcTimeoutBlockCount, swapTimeout: 0})
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

func addDcBills(t *testing.T, w *Wallet, nonce *uint256.Int) {
	addDcBill(t, w, nonce, 1)
	addDcBill(t, w, nonce, 2)
}

func addDcBill(t *testing.T, w *Wallet, nonce *uint256.Int, value uint64) *bill {
	nonceB32 := nonce.Bytes32()
	timeout := uint64(10)
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
	err = w.db.SetBill(&b)
	require.NoError(t, err)
	return &b
}

type metadata struct {
	blockHeight uint64
	dcValueSum  uint64
	dcTimeout   uint64
	swapTimeout uint64
}

func verifyMetadata(t *testing.T, w *Wallet, m metadata) {
	actualBlockHeight, err := w.db.GetBlockHeight()
	require.NoError(t, err)
	require.EqualValues(t, m.blockHeight, actualBlockHeight)

	actualDcValueSum, err := w.db.GetDcValueSum()
	require.NoError(t, err)
	require.EqualValues(t, m.dcValueSum, actualDcValueSum)

	actualDcTimeout, err := w.db.GetDcTimeout()
	require.NoError(t, err)
	require.EqualValues(t, m.dcTimeout, actualDcTimeout)

	actualSwapTimeout, err := w.db.GetSwapTimeout()
	require.NoError(t, err)
	require.EqualValues(t, m.swapTimeout, actualSwapTimeout)
}

func setMetadata(t *testing.T, w *Wallet, m metadata) {
	err := w.db.SetBlockHeight(m.blockHeight)
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

func parseSwapTx(t *testing.T, mockClient *mockAlphaBillClient) (*transaction.Transaction, *transaction.Swap) {
	txSwapPb := mockClient.txs[0]
	txSwap := &transaction.Swap{}
	err := txSwapPb.TransactionAttributes.UnmarshalTo(txSwap)
	require.NoError(t, err)
	return txSwapPb, txSwap
}
