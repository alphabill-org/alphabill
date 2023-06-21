package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletSendFunction(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		balance:   70,
		billId:    uint256.NewInt(0),
		billValue: 50,
		feeCreditBill: &wallet.Bill{
			Id:      []byte{},
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
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
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		balance:   70,
		billId:    uint256.NewInt(0),
		billValue: 50,
		feeCreditBill: &wallet.Bill{
			Id:      []byte{},
			Value:   100 * 1e8,
			TxProof: &wallet.Proof{},
		},
		postTransactionsResponse: map[string]string{"message": "some error"},
	}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)

	// test PostTransactions returns error
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: validPubKey, Amount: amount})
	require.ErrorContains(t, err, "failed to send transactions: status 500 Internal Server Error")
}

func TestWalletSendFunction_WaitForConfirmation(t *testing.T) {
	pubKey := make([]byte, 33)
	b := &Bill{
		Id:      uint256.NewInt(0),
		Value:   100,
		TxHash:  hash.Sum256([]byte{0x01}),
		TxProof: &wallet.Proof{},
	}

	var w *Wallet
	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b}, nil), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			tx := recordedTransactions[0]
			b.TxHash = tx.Hash(crypto.SHA256)
			return createBlockProofResponse(t, b, nil, [][]byte{util.Uint256ToBytes(b.Id)}, dcTimeoutBlockCount, 100, nil), nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      ac.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTransactions = append(recordedTransactions, tx)
			}
			return nil
		},
	}
	w, _ = CreateTestWallet(t, backendMock)

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
	billIds, dcSum := getBillIdsAndSum([]*Bill{b1, b2})
	var w *Wallet
	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b1, b2}, nil), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{
				{Id: b1.GetID(), Value: b1.Value, TxHash: b1.TxHash},
				{Id: b2.GetID(), Value: b2.Value, TxHash: b2.TxHash},
			}, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			var bill *Bill
			for _, tx := range recordedTransactions {
				if bytes.Equal(billId, tx.UnitID()) {
					bill, _ = bills[string(billId)]
					if bill != nil {
						bill.TxHash = tx.Hash(crypto.SHA256)
					}
				}
			}

			if bill != nil {
				return createBlockProofResponse(t, bill, nil, billIds, dcTimeoutBlockCount, dcSum, nil), nil
			} else {
				return nil, errors.New("bill not found")
			}
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8, TxProof: &wallet.Proof{}}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTransactions = append(recordedTransactions, tx)
			}
			return nil
		},
	}
	w, _ = CreateTestWallet(t, backendMock)

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
	billIds, dcSum := getBillIdsAndSum([]*Bill{b1, b2})

	var w *Wallet
	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getRoundNumber: func() (uint64, error) {
			return blockCounter, nil
		},
		listBills: func(pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b1, b2}, nil), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{
				{Id: b1.GetID(), Value: b1.Value, TxHash: b1.TxHash},
				{Id: b2.GetID(), Value: b2.Value, TxHash: b2.TxHash},
			}, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			var bill *Bill
			for _, tx := range recordedTransactions {
				if bytes.Equal(billId, tx.UnitID()) {
					bill, _ = bills[string(billId)]
					if bill != nil {
						bill.TxHash = tx.Hash(crypto.SHA256)
					}
				}
			}
			if bill != nil {
				blockCounter++
				return createBlockProofResponse(t, bill, nil, billIds, dcTimeoutBlockCount, dcSum, nil), nil
			} else {
				return nil, errors.New("bill not found")
			}
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{Id: ac.PrivKeyHash, Value: 100 * 1e8, TxProof: &wallet.Proof{}}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTransactions = append(recordedTransactions, tx)
			}
			return nil
		},
	}
	w, _ = CreateTestWallet(t, backendMock)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: b1.Value + b2.Value, WaitForConfirmation: true})
	require.NoError(t, err)
}

func TestWalletSendFunction_ErrTxFailedToConfirm(t *testing.T) {
	pubKey := make([]byte, 33)
	b := addBill(1)

	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getRoundNumber: func() (uint64, error) {
			if len(recordedTransactions) == 0 {
				return 0, nil
			}
			return 2 * txTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b}, nil), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8, TxProof: &wallet.Proof{}}, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			return &wallet.Bills{Bills: []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTransactions = append(recordedTransactions, tx)
			}
			return nil
		},
	}
	w, _ := CreateTestWallet(t, backendMock)

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
	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills, includeDCMetadata bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*Bill{b}, nil), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8, TxProof: &wallet.Proof{}}, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			return &wallet.Bills{Bills: []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTransactions = append(recordedTransactions, tx)
			}
			return nil
		},
	}
	w, _ := CreateTestWallet(t, backendMock)

	// when whole balance is spent
	_, err := w.Send(context.Background(), SendCmd{ReceiverPubKey: pubKey, Amount: 100})
	require.NoError(t, err)

	// then bill transfer order should be sent
	require.Len(t, recordedTransactions, 1)
	btTx := parseBillTransferTx(t, recordedTransactions[0])
	require.EqualValues(t, 100, btTx.TargetValue)
}
