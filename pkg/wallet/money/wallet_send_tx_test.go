package money

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/holiman/uint256"
	"strings"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

func TestWalletSendFunction(t *testing.T) {
	w, _ := CreateTestWallet(t, &backendMockReturnConf{balance: 70, billId: uint256.NewInt(0), billValue: 50})
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ok response
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.NoError(t, err)
}

func TestWalletSendFunction_InvalidPubKey(t *testing.T) {
	w, _ := CreateTestWallet(t, &backendMockReturnConf{})
	invalidPubKey := make([]byte, 32)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInvalidPubKey
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: invalidPubKey, Amount: amount})
	require.ErrorIs(t, err, ErrInvalidPubKey)
}

func TestWalletSendFunction_InsufficientBalance(t *testing.T) {
	w, _ := CreateTestWallet(t, &backendMockReturnConf{balance: 10})
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInsufficientBalance
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestWalletSendFunction_ClientError(t *testing.T) {
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 70, billId: uint256.NewInt(0), billValue: 50})
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test abclient returns error
	mockClient.SetTxResponse(&txsystem.TransactionResponse{Ok: false, Message: "some error"})
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorContains(t, err, "transaction returned error code: some error")
	mockClient.SetTxResponse(nil)
}

func TestWalletSendFunction_WaitForConfirmation(t *testing.T) {
	pubKey := make([]byte, 33)
	b := &Bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}

	// create block with expected transaction
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 100, billId: b.Id, billTxHash: base64.StdEncoding.EncodeToString(b.TxHash), billValue: b.Value})
	k, _ := w.am.GetAccountKey(0)
	tx, err := createTransaction(pubKey, k, b.Value, b, txTimeoutBlockCount)
	mockClient.SetBlock(&block.Block{BlockNumber: 0, Transactions: []*txsystem.Transaction{
		tx,
	}})

	// test send successfully waits for confirmation
	_, err = w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b.Value, WaitForConfirmation: true, AccountIndex: 0})
	require.NoError(t, err)
	balance, _ := w.GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 100, balance)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmations(t *testing.T) {
	pubKey := make([]byte, 33)
	b1 := addBill(1)
	b2 := addBill(2)
	billsList := ""
	for i := 1; i <= 2; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":"%d","txHash":"%s","isDCBill":false},`, toBillId(uint256.NewInt(uint64(i))), i, base64.StdEncoding.EncodeToString(hash.Sum256([]byte{byte(i)})))
	}
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: fmt.Sprintf(`{"total": 2, "bills": [%s]}`, strings.TrimSuffix(billsList, ","))})

	// create block with expected transactions
	k, _ := w.am.GetAccountKey(0)
	tx1, _ := createTransaction(pubKey, k, b1.Value, b1, txTimeoutBlockCount)
	tx2, _ := createTransaction(pubKey, k, b2.Value, b2, txTimeoutBlockCount)
	mockClient.SetBlock(&block.Block{BlockNumber: 0, Transactions: []*txsystem.Transaction{
		tx2, tx1,
	}})

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b1.Value + b2.Value, WaitForConfirmation: true})
	require.NoError(t, err)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmationsInDifferentBlocks(t *testing.T) {
	pubKey := make([]byte, 33)
	b1 := addBill(1)
	b2 := addBill(2)
	billsList := ""
	for i := 1; i <= 2; i++ {
		billsList = billsList + fmt.Sprintf(`{"id":"%s","value":"%d","txHash":"%s","isDCBill":false},`, toBillId(uint256.NewInt(uint64(i))), i, base64.StdEncoding.EncodeToString(hash.Sum256([]byte{byte(i)})))
	}
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 3, customBillList: fmt.Sprintf(`{"total": 2, "bills": [%s]}`, strings.TrimSuffix(billsList, ","))})

	// create block with expected transactions
	k, _ := w.am.GetAccountKey(0)
	tx1, _ := createTransaction(pubKey, k, b1.Value, b1, txTimeoutBlockCount)
	tx2, _ := createTransaction(pubKey, k, b2.Value, b2, txTimeoutBlockCount)
	mockClient.SetBlock(&block.Block{BlockNumber: 0, Transactions: []*txsystem.Transaction{
		tx1,
	}})
	mockClient.SetBlock(&block.Block{BlockNumber: 1, Transactions: []*txsystem.Transaction{
		tx2,
	}})

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b1.Value + b2.Value, WaitForConfirmation: true})
	require.NoError(t, err)
}

func TestWalletSendFunction_ErrTxFailedToConfirm(t *testing.T) {
	pubKey := make([]byte, 33)
	b := addBill(1)
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 10, billId: b.Id, billValue: b.Value})

	for i := 0; i <= txTimeoutBlockCount; i++ {
		mockClient.SetBlock(&block.Block{BlockNumber: uint64(i)})
	}

	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b.Value, WaitForConfirmation: true})
	require.ErrorIs(t, err, ErrTxFailedToConfirm)
}

func TestWholeBalanceIsSentUsingBillTransferOrder(t *testing.T) {
	// create wallet with single bill
	pubKey := make([]byte, 33)
	b := &Bill{
		Id:     uint256.NewInt(1),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value})

	// when whole balance is spent
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: 100})
	require.NoError(t, err)

	// then bill transfer order should be sent
	require.Len(t, mockClient.GetRecordedTransactions(), 1)
	btTx := parseBillTransferTx(t, mockClient.GetRecordedTransactions()[0])
	require.EqualValues(t, 100, btTx.TargetValue)
}

func TestWalletSendFunction_RetryTxWhenTxBufferIsFull(t *testing.T) {
	// setup wallet
	b := &Bill{
		Id:     uint256.NewInt(1),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value})

	// make server return TxBufferFullErrMessage
	mockClient.SetTxResponse(&txsystem.TransactionResponse{Ok: false, Message: txBufferFullErrMsg})

	// send tx
	wg := sync.WaitGroup{}
	wg.Add(1)
	var sendError error
	go func() {
		_, sendError = w.Send(context.Background(), SendCmd{ReceiverPubKey: make([]byte, 33), Amount: 50})
		wg.Done()
	}()

	// verify txs are broadcasted multiple times
	require.Eventually(t, func() bool {
		return len(mockClient.GetRecordedTransactions()) == maxTxFailedTries
	}, test.WaitDuration, test.WaitTick)

	// wait for send goroutine to finish
	wg.Wait()

	// and verify send tx error
	require.ErrorIs(t, sendError, wallet.ErrFailedToBroadcastTx)
}

func TestWalletSendFunction_RetryCanBeCanceledByUser(t *testing.T) {
	// setup wallet
	b := &Bill{
		Id:     uint256.NewInt(1),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value})

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
