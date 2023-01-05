package money

import (
	"context"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletSendFunction(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	invalidPubKey := make([]byte, 32)
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInvalidPubKey
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: invalidPubKey, Amount: amount})
	require.ErrorIs(t, err, ErrInvalidPubKey)

	// test ErrInsufficientBalance
	_, err = w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorIs(t, err, ErrInsufficientBalance)

	// test abclient returns error
	b := Bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	err = w.db.Do().SetBill(0, &b)
	require.NoError(t, err)
	mockClient.SetTxResponse(&txsystem.TransactionResponse{Ok: false, Message: "some error"})
	_, err = w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorContains(t, err, "transaction returned error code: some error")
	mockClient.SetTxResponse(nil)

	// test ErrSwapInProgress
	nonce := calculateExpectedDcNonce(t, w)
	setDcMetadata(t, w, nonce, &dcMetadata{DcValueSum: 101, DcTimeout: dcTimeoutBlockCount})
	_, err = w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorIs(t, err, ErrSwapInProgress)
	setDcMetadata(t, w, nonce, nil)

	// test ok response
	_, err = w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.NoError(t, err)

	// test another account
	_, _, _ = w.AddAccount()
	_ = w.db.Do().SetBill(1, &Bill{Id: uint256.NewInt(55555), Value: 50})
	_, err = w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount, AccountIndex: 1})
	require.NoError(t, err)
}

func TestWalletSendFunction_WaitForConfirmation(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	pubKey := make([]byte, 33)
	b := &Bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	_ = w.db.Do().SetBill(0, b)

	// create block with expected transaction
	k, _ := w.db.Do().GetAccountKey(0)
	tx, err := createTransaction(pubKey, k, b.Value, b, txTimeoutBlockCount)
	mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{
		tx,
	}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 0}}})

	// verify balance before transaction
	balance, _ := w.db.Do().GetBalance(GetBalanceCmd{})
	require.Equal(t, b.Value, balance)

	// test send successfully waits for confirmation
	_, err = w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b.Value, WaitForConfirmation: true, AccountIndex: 0})
	require.NoError(t, err)
	balance, _ = w.db.Do().GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 0, balance)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmations(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	pubKey := make([]byte, 33)
	b1 := &Bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	b2 := &Bill{
		Id:     uint256.NewInt(2),
		Value:  2,
		TxHash: hash.Sum256([]byte{0x02}),
	}
	_ = w.db.Do().SetBill(0, b1)
	_ = w.db.Do().SetBill(0, b2)

	// create block with expected transactions
	k, _ := w.db.Do().GetAccountKey(0)
	tx1, _ := createTransaction(pubKey, k, b1.Value, b1, txTimeoutBlockCount)
	tx2, _ := createTransaction(pubKey, k, b2.Value, b2, txTimeoutBlockCount)
	mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{
		tx2, tx1,
	}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 0}}})

	// verify balance before transactions
	balance, _ := w.db.Do().GetBalance(GetBalanceCmd{})
	require.EqualValues(t, b1.Value+b2.Value, balance)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b1.Value + b2.Value, WaitForConfirmation: true})
	require.NoError(t, err)

	// verify balance after transactions
	balance, _ = w.db.Do().GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 0, balance)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmationsInDifferentBlocks(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	pubKey := make([]byte, 33)
	b1 := &Bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	b2 := &Bill{
		Id:     uint256.NewInt(2),
		Value:  2,
		TxHash: hash.Sum256([]byte{0x02}),
	}
	_ = w.db.Do().SetBill(0, b1)
	_ = w.db.Do().SetBill(0, b2)

	// create block with expected transactions
	k, _ := w.db.Do().GetAccountKey(0)
	tx1, _ := createTransaction(pubKey, k, b1.Value, b1, txTimeoutBlockCount)
	tx2, _ := createTransaction(pubKey, k, b2.Value, b2, txTimeoutBlockCount)
	mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{
		tx1,
	}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 0}}})
	mockClient.SetBlock(&block.Block{Transactions: []*txsystem.Transaction{
		tx2,
	}, UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}}})

	// verify balance before transactions
	balance, _ := w.db.Do().GetBalance(GetBalanceCmd{})
	require.EqualValues(t, b1.Value+b2.Value, balance)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b1.Value + b2.Value, WaitForConfirmation: true})

	require.NoError(t, err)

	// verify balance after transactions
	balance, _ = w.db.Do().GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 0, balance)
}

func TestWalletSendFunction_ErrTxFailedToConfirm(t *testing.T) {
	w, mockClient := CreateTestWallet(t)
	pubKey := make([]byte, 33)
	b := &Bill{
		Id:     uint256.NewInt(1),
		Value:  1,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	_ = w.db.Do().SetBill(0, b)

	for i := 0; i <= txTimeoutBlockCount; i++ {
		mockClient.SetBlock(&block.Block{UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: uint64(i)}}})
	}

	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b.Value, WaitForConfirmation: true})
	require.ErrorIs(t, err, ErrTxFailedToConfirm)
}

func TestWholeBalanceIsSentUsingBillTransferOrder(t *testing.T) {
	// create wallet with single bill
	w, mockClient := CreateTestWallet(t)
	addBill(t, w, 100)
	pubkey := make([]byte, 33)

	// when whole balance is spent
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubkey, Amount: 100})
	require.NoError(t, err)

	// then bill transfer order should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	btTx := parseBillTransferTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 100, btTx.TargetValue)
}

func TestWalletSendFunction_RetryTxWhenTxBufferIsFull(t *testing.T) {
	// setup wallet
	w, mockClient := CreateTestWallet(t)
	b := Bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	_ = w.db.Do().SetBill(0, &b)

	// make server return TxBufferFullErrMessage
	mockClient.SetTxResponse(&txsystem.TransactionResponse{Ok: false, Message: txBufferFullErrMsg})

	// send tx
	_, sendError := w.Send(context.Background(), SendCmd{ReceiverPubKey: make([]byte, 33), Amount: 50})

	// verify send tx error
	require.ErrorIs(t, sendError, wallet.ErrFailedToBroadcastTx)

	// verify txs were broadcasted multiple times
	require.Eventually(t, func() bool {
		return len(mockClient.GetRecordedTransactions()) == maxTxFailedTries
	}, test.WaitDuration, test.WaitTick)
}

func TestWalletSendFunction_RetryCanBeCanceledByUser(t *testing.T) {
	// setup wallet
	w, mockClient := CreateTestWallet(t)
	b := Bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	_ = w.db.Do().SetBill(0, &b)

	// make server return TxBufferFullErrMessage
	mockClient.SetTxResponse(&txsystem.TransactionResponse{Ok: false, Message: txBufferFullErrMsg})

	// send tx
	ctx, cancel := context.WithCancel(context.Background())
	wg := sync.WaitGroup{}
	wg.Add(1)
	var sendError error
	go func() {
		_, sendError = w.Send(ctx, SendCmd{ReceiverPubKey: make([]byte, 33), Amount: 50})
		wg.Done()
	}()

	// when context is canceled
	cancel()

	// then sendError returns immediately
	wg.Wait()
	require.ErrorIs(t, sendError, wallet.ErrTxRetryCanceled)

	// and only the initial transaction should be broadcast
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
}
