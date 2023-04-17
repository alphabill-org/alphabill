package money

import (
	"bytes"
	"context"
	"crypto"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
)

func TestWalletSendFunction(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 70, billId: uint256.NewInt(0), billValue: 50}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ok response
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.NoError(t, err)
}

func TestWalletSendFunction_InvalidPubKey(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{}))
	invalidPubKey := make([]byte, 32)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInvalidPubKey
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: invalidPubKey, Amount: amount})
	require.ErrorIs(t, err, ErrInvalidPubKey)
}

func TestWalletSendFunction_InsufficientBalance(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 10}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInsufficientBalance
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorIs(t, err, ErrInsufficientBalance)
}

func TestWalletSendFunction_ClientError(t *testing.T) {
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 70, billId: uint256.NewInt(0), billValue: 50}))
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

	var mockClient *clientmock.MockAlphabillClient
	backend := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getBlockHeight: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte) (*money.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b}), nil
		},
		getProof: func(billId []byte) (*block.Bills, error) {
			tx := mockClient.GetRecordedTransactions()[0]
			gtx, err := txConverter.ConvertTx(tx)
			require.NoError(t, err)
			b.TxHash = gtx.Hash(crypto.SHA256)
			return createBlockProofResponse(t, b, nil, 0, dcTimeoutBlockCount), nil
		},
	}
	w, mockClient := CreateTestWallet(t, backend)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b.Value, WaitForConfirmation: true, AccountIndex: 0})
	require.NoError(t, err)
	balance, _ := w.GetBalance(GetBalanceCmd{})
	require.EqualValues(t, 100, balance)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmations(t *testing.T) {
	pubKey := make([]byte, 33)
	b1 := addBill(1)
	b2 := addBill(2)
	bills := map[string]*Bill{
		string(util.Uint256ToBytes(b1.Id)): b1,
		string(util.Uint256ToBytes(b2.Id)): b2,
	}

	var mockClient *clientmock.MockAlphabillClient
	backend := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getBlockHeight: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte) (*money.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b1, b2}), nil
		},
		getProof: func(billId []byte) (*block.Bills, error) {
			txs := mockClient.GetRecordedTransactions()
			var bill *Bill
			for _, tx := range txs {
				if bytes.Equal(billId, tx.UnitId) {
					bill, _ = bills[string(billId)]
					if bill != nil {
						gtx, err := txConverter.ConvertTx(tx)
						require.NoError(t, err)
						bill.TxHash = gtx.Hash(crypto.SHA256)
					}
				}
			}

			if bill != nil {
				return createBlockProofResponse(t, bill, nil, 0, dcTimeoutBlockCount), nil
			} else {
				return nil, errors.New("bill not found")
			}
		},
	}
	w, mockClient := CreateTestWallet(t, backend)

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
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 3, customBillList: fmt.Sprintf(`{"total": 2, "bills": [%s]}`, strings.TrimSuffix(billsList, ","))}))

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
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 10, billId: b.Id, billValue: b.Value}))

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
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value}))

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
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value}))

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
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value}))

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
