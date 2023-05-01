package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
)

func TestWalletSendFunction(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		balance:   70,
		billId:    uint256.NewInt(0),
		billValue: 50,
		feeCreditBill: &bp.Bill{
			Id:      []byte{},
			Value:   100 * 1e8,
			TxProof: &block.TxProof{},
		}}))
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
	mockClient.SetTxResponse(errors.New("some error"))
	_, err := w.Send(ctx, SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorContains(t, err, "failed to send transaction: some error")
}

func TestWalletSendFunction_WaitForConfirmation(t *testing.T) {
	pubKey := make([]byte, 33)
	b := &Bill{
		Id:     uint256.NewInt(0),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}

	var w *Wallet
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
		getProof: func(billId []byte) (*bp.Bills, error) {
			tx := mockClient.GetRecordedTransactions()[0]
			gtx, err := NewTxConverter(w.SystemID()).ConvertTx(tx)
			require.NoError(t, err)
			b.TxHash = gtx.Hash(crypto.SHA256)
			return createBlockProofResponse(t, b, nil, 0, dcTimeoutBlockCount, nil), nil
		},
	}
	w, mockClient = CreateTestWallet(t, backend)

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
	var w *Wallet
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
		getProof: func(billId []byte) (*bp.Bills, error) {
			txs := mockClient.GetRecordedTransactions()
			var bill *Bill
			for _, tx := range txs {
				if bytes.Equal(billId, tx.UnitId) {
					bill, _ = bills[string(billId)]
					if bill != nil {
						gtx, err := NewTxConverter(w.SystemID()).ConvertTx(tx)
						require.NoError(t, err)
						bill.TxHash = gtx.Hash(crypto.SHA256)
					}
				}
			}

			if bill != nil {
				return createBlockProofResponse(t, bill, nil, 0, dcTimeoutBlockCount, nil), nil
			} else {
				return nil, errors.New("bill not found")
			}
		},
	}
	w, mockClient = CreateTestWallet(t, backend)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b1.Value + b2.Value, WaitForConfirmation: true})
	require.NoError(t, err)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmationsInDifferentBlocks(t *testing.T) {
	pubKey := make([]byte, 33)
	b1 := addBill(1)
	b2 := addBill(2)

	blockCounter := uint64(0)

	bills := map[string]*Bill{
		string(util.Uint256ToBytes(b1.Id)): b1,
		string(util.Uint256ToBytes(b2.Id)): b2,
	}
	var w *Wallet
	var mockClient *clientmock.MockAlphabillClient
	backend := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getBlockHeight: func() (uint64, error) {
			return blockCounter, nil
		},
		listBills: func(pubKey []byte) (*money.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b1, b2}), nil
		},
		getProof: func(billId []byte) (*bp.Bills, error) {
			txs := mockClient.GetRecordedTransactions()
			var bill *Bill
			for _, tx := range txs {
				if bytes.Equal(billId, tx.UnitId) {
					bill, _ = bills[string(billId)]
					if bill != nil {
						gtx, err := NewTxConverter(w.SystemID()).ConvertTx(tx)
						require.NoError(t, err)
						bill.TxHash = gtx.Hash(crypto.SHA256)
					}
				}
			}

			if bill != nil {
				nr := blockCounter
				blockCounter++
				return createBlockProofResponse(t, bill, nil, nr, dcTimeoutBlockCount, nil), nil
			} else {
				return nil, errors.New("bill not found")
			}
		},
	}
	w, mockClient = CreateTestWallet(t, backend)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b1.Value + b2.Value, WaitForConfirmation: true})
	require.NoError(t, err)
}

func TestWalletSendFunction_ErrTxFailedToConfirm(t *testing.T) {
	pubKey := make([]byte, 33)
	b := addBill(1)
	//w, mockClient := CreateTestWallet(t, &backendMockReturnConf{
	//	balance:   10,
	//	billId:    b.Id,
	//	billValue: b.Value,
	//	feeCreditBill: &bp.Bill{
	//		Id:      []byte{},
	//		Value:   100 * 1e8,
	//		TxProof: &block.TxProof{},
	//	},
	//})

	var mockClient *clientmock.MockAlphabillClient
	backend := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getBlockHeight: func() (uint64, error) {
			if len(mockClient.GetRecordedTransactions()) == 0 {
				return 0, nil
			}
			return 2 * txTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte) (*money.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b}), nil
		},
	}
	w, mockClient := CreateTestWallet(t, backend)

	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b.Value, WaitForConfirmation: true})
	require.ErrorContains(t, err, "confirmation timeout")
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
	//w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value, // TODO: pg
	//	feeCreditBill: &bp.Bill{
	//		Id:      []byte{},
	//		Value:   100 * 1e8,
	//		TxProof: &block.TxProof{},
	//	}})

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
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value})) // TODO: pg
	//w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value, feeCreditBill: &bp.Bill{
	//	Id:      []byte{},
	//	Value:   100 * 1e8,
	//	TxProof: &block.TxProof{},
	//}})

	// make server return TxBufferFullErrMessage
	mockClient.SetTxResponse(errors.New(txBufferFullErrMsg))

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
	b := &Bill{
		Id:     uint256.NewInt(1),
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}
	w, mockClient := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value}))
	//w, mockClient := CreateTestWallet(t, &backendMockReturnConf{balance: 100, billId: b.Id, billValue: b.Value, feeCreditBill: &bp.Bill{
	//	Id:      []byte{},
	//	Value:   100 * 1e8,
	//	TxProof: &block.TxProof{},
	//}})

	// make server return TxBufferFullErrMessage
	mockClient.SetTxResponse(errors.New(txBufferFullErrMsg))

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
