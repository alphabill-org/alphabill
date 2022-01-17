package wallet

import (
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/internal/testutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestSwapIsTriggeredWhenDcSumIsReached(t *testing.T) {
	w, mockClient := createTestWallet(t)
	addBills(t, w)

	// when dc runs
	err := w.collectDust()
	require.NoError(t, err)

	// then metadata is updated
	verifyMetadata(t, w, 0, 3, 10, 0)

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
	verifyMetadata(t, w, 1, 0, 0, 61)

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
	for i := 0; i < 9; i++ {
		block := &alphabill.Block{
			BlockNo:            uint64(2 + i),
			PrevBlockHash:      hash.Sum256([]byte{}),
			Transactions:       []*transaction.Transaction{},
			UnicityCertificate: []byte{},
		}
		err = w.processBlock(block)
		require.NoError(t, err)
	}

	// then no more swap txs should be triggered
	require.Len(t, mockClient.txs, 3) // 2 dc + 1 swap

	// and metadata is updated
	verifyMetadata(t, w, 10, 0, 0, 61)

	// when swap tx block is received
	err = w.db.SetBlockHeight(60)
	require.NoError(t, err)
	block = &alphabill.Block{
		BlockNo:            uint64(61),
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs[2:3], // swap tx
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then metadata is updated
	verifyMetadata(t, w, 61, 0, 0, 0)
	verifyDcNonce(t, w, nil)
	verifyBalance(t, w, 3)
}

func TestDcTimeoutIsReachedWithPartialConfirmedDcTxs(t *testing.T) {
	w, mockClient := createTestWallet(t)
	addBills(t, w)

	// when dc runs
	err := w.collectDust()
	require.NoError(t, err)

	// then metadata is updated
	verifyMetadata(t, w, 0, 3, 10, 0)

	// and two dc txs are broadcast
	require.Len(t, mockClient.txs, 2)

	// when block with 1 of the 2 dc txs is received
	block := &alphabill.Block{
		BlockNo:            1,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs[0:1],
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then corresponding bill gets marked as dc bill
	b1, err := w.db.GetBill(uint256.NewInt(1))
	require.NoError(t, err)
	require.True(t, b1.IsDcBill)

	// and bill 2 remains non dc bill
	b2, err := w.db.GetBill(uint256.NewInt(2))
	require.NoError(t, err)
	require.False(t, b2.IsDcBill)

	// and dc nonce exists
	verifyDcNonceExists(t, w)

	// when dcTimeout is reached
	err = w.db.SetBlockHeight(9)
	require.NoError(t, err)
	block = &alphabill.Block{
		BlockNo:            uint64(10),
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then swap should not be triggered
	height, err := w.db.GetBlockHeight()
	require.EqualValues(t, 10, height)
	require.NoError(t, err)
	require.Len(t, mockClient.txs, 2) // 2 dc (1 lost)

	// and metadata is updated
	verifyMetadata(t, w, 10, 0, 0, 0)
	verifyBalance(t, w, 3)
	verifyDcNonceExists(t, w)
}

func TestDcTimeoutIsReachedWithoutAnyDcTxs(t *testing.T) {
	w, mockClient := createTestWallet(t)
	addBills(t, w)

	// when collect dust is called
	err := w.collectDust()
	require.NoError(t, err)

	// then dust txs are broadcast
	require.Len(t, mockClient.txs, 2) // 2 dc txs

	// and metadata is updated
	verifyMetadata(t, w, 0, 3, 10, 0)
	verifyBalance(t, w, 3)

	// when dcTimeout is reached without any confirmed dust transfers
	err = w.db.SetBlockHeight(9)
	require.NoError(t, err)
	block := &alphabill.Block{
		BlockNo:            uint64(10),
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)

	// then no error must be thrown
	require.NoError(t, err)

	// and no swap tx must be broadcast
	require.Len(t, mockClient.txs, 2) // 2 dc txs

	// and metadata is updated
	verifyMetadata(t, w, 10, 0, 0, 0)
	verifyBalance(t, w, 3)
	verifyDcNonce(t, w, nil) // no dc blocks were processed
}

func TestSwapTimeoutIsReachedWithConfirmedDcTxs(t *testing.T) {
	w, mockClient := createTestWallet(t)
	addBills(t, w)

	// when dc runs
	err := w.collectDust()
	require.NoError(t, err)

	// then metadata is updated
	verifyMetadata(t, w, 0, 3, 10, 0)

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
	verifyMetadata(t, w, 1, 0, 0, 61)
	verifyDcNonceExists(t, w)

	// when swap timeout is reached
	err = w.db.SetBlockHeight(60)
	require.NoError(t, err)
	block = &alphabill.Block{
		BlockNo:            uint64(61),
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       []*transaction.Transaction{},
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then metadata is updated
	verifyMetadata(t, w, 61, 0, 0, 0)

	// and dc bills remain in the wallet
	bills, err := w.db.GetBills()
	require.NoError(t, err)
	require.Len(t, bills, 2)
	for _, b := range bills {
		require.True(t, b.IsDcBill)
		require.NotNil(t, b.DcTx)
		require.NotNil(t, b.DcNonce)
	}
}

func TestConfirmedDcBillsAreUsedInNewDcProcess(t *testing.T) {
	w, _ := createTestWallet(t)
	addDcBills(t, w)
	setMetadata(t, w, 60, 0, 0, 0)

	// when dust collector runs and only confirmed dc bills exist
	err := w.collectDust()
	require.NoError(t, err)

	// then metadata is updated
	verifyMetadata(t, w, 60, 3, 70, 0)
	verifyDcNonce(t, w, uint256.NewInt(0))

}

func createTestWallet(t *testing.T) (*Wallet, *mockAlphaBillClient) {
	_ = testutil.DeleteWalletDb(os.TempDir())
	c := &Config{DbPath: os.TempDir()}
	w, err := CreateNewWallet(c)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	mockClient := &mockAlphaBillClient{}
	w.alphaBillClient = mockClient
	return w, mockClient
}

func addBills(t *testing.T, w *Wallet) {
	// add two bills to wallet
	b1 := bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err := w.db.SetBill(&b1)
	require.NoError(t, err)

	b2 := bill{
		Id:     uint256.NewInt(2),
		Value:  2,
		TxHash: hash.Sum256([]byte{0x02}),
	}
	err = w.db.SetBill(&b2)
	require.NoError(t, err)
}

func addDcBills(t *testing.T, w *Wallet) {
	// add two confirmed dc bills to wallet
	nonce := uint256.NewInt(0)
	nonceB32 := nonce.Bytes32()
	timeout := uint64(10)
	b1 := bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	tx, err := w.createDustTx(&b1, nonce, timeout)
	require.NoError(t, err)
	b1.IsDcBill = true
	b1.DcTx = tx
	b1.DcNonce = nonceB32[:]
	err = w.db.SetBill(&b1)
	require.NoError(t, err)

	b2 := bill{
		Id:     uint256.NewInt(2),
		Value:  2,
		TxHash: hash.Sum256([]byte{0x02}),
	}
	tx, err = w.createDustTx(&b2, nonce, timeout)
	require.NoError(t, err)
	b2.IsDcBill = true
	b2.DcTx = tx
	b2.DcNonce = nonceB32[:]
	err = w.db.SetBill(&b2)
	require.NoError(t, err)
}

func verifyMetadata(t *testing.T, w *Wallet, blockHeight uint64, dcValueSum uint64, dcTimeout uint64, swapTimeout uint64) {
	actualBlockHeight, err := w.db.GetBlockHeight()
	require.NoError(t, err)
	require.EqualValues(t, blockHeight, actualBlockHeight)

	actualDcValueSum, err := w.db.GetDcValueSum()
	require.NoError(t, err)
	require.EqualValues(t, dcValueSum, actualDcValueSum)

	actualDcTimeout, err := w.db.GetDcTimeout()
	require.NoError(t, err)
	require.EqualValues(t, dcTimeout, actualDcTimeout)

	actualSwapTimeout, err := w.db.GetSwapTimeout()
	require.NoError(t, err)
	require.EqualValues(t, swapTimeout, actualSwapTimeout)
}

func setMetadata(t *testing.T, w *Wallet, blockHeight uint64, dcValueSum uint64, dcTimeout uint64, swapTimeout uint64) {
	err := w.db.SetBlockHeight(blockHeight)
	require.NoError(t, err)

	err = w.db.SetDcValueSum(dcValueSum)
	require.NoError(t, err)

	err = w.db.SetDcTimeout(dcTimeout)
	require.NoError(t, err)

	err = w.db.SetSwapTimeout(swapTimeout)
	require.NoError(t, err)
}

func verifyDcNonce(t *testing.T, w *Wallet, dcNonce *uint256.Int) {
	actualDcNonce, err := w.db.GetDcNonce()
	require.NoError(t, err)
	require.EqualValues(t, dcNonce, actualDcNonce)
}

func verifyDcNonceExists(t *testing.T, w *Wallet) {
	dcNonce, err := w.db.GetDcNonce()
	require.NoError(t, err)
	require.NotNil(t, dcNonce)
}

func verifyBalance(t *testing.T, w *Wallet, balance uint64) {
	actualDcNonce, err := w.db.GetBalance()
	require.NoError(t, err)
	require.EqualValues(t, balance, actualDcNonce)
}
