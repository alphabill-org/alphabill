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
	// setup wallet
	testutil.DeleteWalletDb(os.TempDir())
	c := &Config{DbPath: os.TempDir()}
	w, err := CreateNewWallet(c)
	defer DeleteWallet(w)
	require.NoError(t, err)

	mockClient := &mockAlphaBillClient{}
	w.alphaBillClient = mockClient

	// add two bills to wallet
	b1 := bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.SetBill(&b1)
	require.NoError(t, err)

	b2 := bill{
		Id:     uint256.NewInt(2),
		Value:  2,
		TxHash: hash.Sum256([]byte{0x02}),
	}
	err = w.db.SetBill(&b2)
	require.NoError(t, err)

	// when dc runs
	err = w.collectDust()
	require.NoError(t, err)

	// then dc block height is recorded
	dcBlockHeight, err := w.db.GetDcBlockHeight()
	require.NoError(t, err)
	require.EqualValues(t, 10, dcBlockHeight)

	// and dc value sum is recorded
	dcValueSum, err := w.db.GetDcValueSum()
	require.NoError(t, err)
	require.EqualValues(t, 3, dcValueSum)

	// and two dc txs are broadcast
	require.Len(t, mockClient.txs, 2)

	// when block with dc txs is received
	block := &alphabill.Block{
		BlockNo:            1,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs,
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then block height is updated
	height, err := w.db.GetBlockHeight()
	require.EqualValues(t, 1, height)
	require.NoError(t, err)

	// and swap tx is broadcast
	require.Len(t, mockClient.txs, 3) // 2 dc + 1 swap
	txSwapPb := mockClient.txs[2]
	txSwap := &transaction.Swap{}
	err = txSwapPb.TransactionAttributes.UnmarshalTo(txSwap)
	require.NoError(t, err)

	// and swap tx contains the exact same individual dc txs
	for i := 0; i < len(txSwap.DustTransferOrders); i++ {
		mockClientTx := mockClient.txs[i]
		dustTransferTx := &transaction.DustTransfer{}
		err = mockClientTx.TransactionAttributes.UnmarshalTo(dustTransferTx)
		require.NoError(t, err)

		dustTransferSwapTx := txSwap.DustTransferOrders[i]
		dustTransferTxInSwap := &transaction.DustTransfer{}
		err = dustTransferSwapTx.TransactionAttributes.UnmarshalTo(dustTransferTxInSwap)
		require.NoError(t, err)
		require.EqualValues(t, dustTransferTx.NewBearer, dustTransferTxInSwap.NewBearer)
		require.EqualValues(t, dustTransferTx.TargetValue, dustTransferTxInSwap.TargetValue)
		require.EqualValues(t, dustTransferTx.Backlink, dustTransferTxInSwap.Backlink)
		require.EqualValues(t, dustTransferTx.Nonce, dustTransferTxInSwap.Nonce)
	}

	// then dcBlockHeight is cleared
	dcBlockHeight, err = w.db.GetDcBlockHeight()
	require.NoError(t, err)
	require.EqualValues(t, 0, dcBlockHeight)

	// and dcValueSum is cleared
	dcValueSum, err = w.db.GetDcValueSum()
	require.NoError(t, err)
	require.EqualValues(t, 0, dcValueSum)

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

	// then swap should not be triggered and no errors should be thrown
	require.Len(t, mockClient.txs, 3) // 2 dc + 1 swap
}

func TestSwapIsTriggeredWhenDcBlockHeightIsReached(t *testing.T) {
	// setup wallet
	testutil.DeleteWalletDb(os.TempDir())
	c := &Config{DbPath: os.TempDir()}
	w, err := CreateNewWallet(c)
	defer DeleteWallet(w)
	require.NoError(t, err)

	mockClient := &mockAlphaBillClient{}
	w.alphaBillClient = mockClient

	// add two bills
	b1 := bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.SetBill(&b1)
	require.NoError(t, err)

	b2 := bill{
		Id:     uint256.NewInt(2),
		Value:  2,
		TxHash: hash.Sum256([]byte{0x02}),
	}
	err = w.db.SetBill(&b2)
	require.NoError(t, err)

	balance, err := w.GetBalance()
	require.NoError(t, err)
	require.EqualValues(t, 3, balance)

	// when dc runs
	err = w.collectDust()
	require.NoError(t, err)

	// then dc block height is recorded
	dcBlockHeight, err := w.db.GetDcBlockHeight()
	require.NoError(t, err)
	require.EqualValues(t, 10, dcBlockHeight)

	// and dc bills value sum is recorded
	dcValueSum, err := w.db.GetDcValueSum()
	require.NoError(t, err)
	require.EqualValues(t, 3, dcValueSum)

	// and two dc txs are broadcast
	require.Len(t, mockClient.txs, 2)

	// when block with 1 of 2 dc txs is received
	block := &alphabill.Block{
		BlockNo:            1,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs[0:1],
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then corresponding bill gets marked as dc bill
	bill1, err := w.db.GetBill(b1.Id)
	require.NoError(t, err)
	require.True(t, bill1.IsDcBill)

	// when receiving empty blocks until reaching dcBlockHeight
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

	// then wallet should have triggered swap
	height, err := w.db.GetBlockHeight()
	require.EqualValues(t, 10, height)
	require.NoError(t, err)
	require.Len(t, mockClient.txs, 3) // 2 dc (1 lost) + 1 swap

	// then dc block height is cleared
	dcBlockHeight, err = w.db.GetDcBlockHeight()
	require.NoError(t, err)
	require.EqualValues(t, 0, dcBlockHeight)

	// and dc bills value sum is cleared
	dcValueSum, err = w.db.GetDcValueSum()
	require.NoError(t, err)
	require.EqualValues(t, 0, dcValueSum)

	// when the block with swap tx is received
	block = &alphabill.Block{
		BlockNo:            11,
		PrevBlockHash:      hash.Sum256([]byte{}),
		Transactions:       mockClient.txs[2:3],
		UnicityCertificate: []byte{},
	}
	err = w.processBlock(block)
	require.NoError(t, err)

	// then wallet should contain two bills
	bills, err := w.db.GetBills()
	require.NoError(t, err)
	require.Len(t, bills, 2)

	// and neither should be marked as dc bill
	for _, b := range bills {
		require.False(t, b.IsDcBill)
	}

	// and have the same balance as before
	balance, err = w.db.GetBalance()
	require.EqualValues(t, 3, balance)

	// and no new transactions must be published
	require.Len(t, mockClient.txs, 3) // 2 dc (1 lost) + 1 swap
}

func TestDcBlockHeightIsReachedWithoutAnyConfirmedDustTransfers(t *testing.T) {
	// setup wallet
	testutil.DeleteWalletDb(os.TempDir())
	c := &Config{DbPath: os.TempDir()}
	w, err := CreateNewWallet(c)
	defer DeleteWallet(w)
	require.NoError(t, err)

	mockClient := &mockAlphaBillClient{}
	w.alphaBillClient = mockClient

	// add two bills
	b1 := bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.SetBill(&b1)
	require.NoError(t, err)

	b2 := bill{
		Id:     uint256.NewInt(2),
		Value:  2,
		TxHash: hash.Sum256([]byte{0x02}),
	}
	err = w.db.SetBill(&b2)
	require.NoError(t, err)

	// set dcBlockHeight
	err = w.db.SetDcBlockHeight(10)
	require.NoError(t, err)

	// set dvValueSum
	err = w.db.SetDcValueSum(3)
	require.NoError(t, err)

	// set block height
	err = w.db.SetBlockHeight(9)
	require.NoError(t, err)

	// when dcBlockHeight is reached without any confirmed  dust transfers
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
	require.Len(t, mockClient.txs, 0)
}
