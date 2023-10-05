package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

func TestWalletSendFunction_Ok(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		balance:   70,
		billId:    money.NewBillID(nil, []byte{0}),
		billValue: 50,
		feeCreditBill: &wallet.Bill{
			Id:    []byte{},
			Value: 100 * 1e8,
		}}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ok response
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.NoError(t, err)
}

func TestWalletSendFunction_InvalidPubKey(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{}))
	invalidPubKey := make([]byte, 32)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInvalidPubKey
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: invalidPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "invalid public key: public key must be in compressed secp256k1 format: got 32 "+
		"bytes, expected 33 bytes for public key 0x0000000000000000000000000000000000000000000000000000000000000000")
}

func TestWalletSendFunction_InsufficientBalance(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{balance: 10}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInsufficientBalance
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

func TestWalletSendFunction_ClientError(t *testing.T) {
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		balance:   70,
		billId:    money.NewBillID(nil, []byte{0}),
		billValue: 50,
		feeCreditBill: &wallet.Bill{
			Id:    []byte{},
			Value: 100 * 1e8,
		},
		postTransactionsResponse: map[string]string{"message": "some error"},
	}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)

	// test PostTransactions returns error
	_, err := w.Send(context.Background(), SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "failed to send transactions: status 500 Internal Server Error")
}

func TestWalletSendFunction_WaitForConfirmation(t *testing.T) {
	pubKey := make([]byte, 33)
	b := &wallet.Bill{
		Id:     []byte{0},
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
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
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b}), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx := recordedTransactions[0]
			b.TxHash = tx.Hash(crypto.SHA256)
			return createBlockProofResponseForDustTransfer(t, b, nil, txTimeoutBlockCount, nil), nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w, _ = CreateTestWallet(t, backendMock)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: pubKey, Amount: b.Value}},
		WaitForConfirmation: true,
	})
	require.NoError(t, err)
	balance, _ := w.GetBalance(context.Background(), GetBalanceCmd{})
	require.EqualValues(t, 100, balance)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmations(t *testing.T) {
	pubKey := make([]byte, 33)
	b1 := createBill(1)
	b2 := createBill(2)
	bills := map[string]*wallet.Bill{
		string(b1.Id): b1,
		string(b2.Id): b2,
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
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b1, b2}), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{
				{Id: b1.GetID(), Value: b1.Value, TxHash: b1.TxHash},
				{Id: b2.GetID(), Value: b2.Value, TxHash: b2.TxHash},
			}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			var bill *wallet.Bill
			for _, tx := range recordedTransactions {
				if bytes.Equal(unitID, tx.UnitID()) {
					bill = bills[string(unitID)]
					if bill != nil {
						bill.TxHash = tx.Hash(crypto.SHA256)
					}
				}
			}

			if bill != nil {
				return createBlockProofResponseForDustTransfer(t, bill, nil, txTimeoutBlockCount, nil), nil
			} else {
				return nil, errors.New("bill not found")
			}
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w, _ = CreateTestWallet(t, backendMock)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: pubKey, Amount: b1.Value + b2.Value}},
		WaitForConfirmation: true,
	})
	require.NoError(t, err)
}

func TestWalletSendFunction_WaitForMultipleTxConfirmationsInDifferentBlocks(t *testing.T) {
	pubKey := make([]byte, 33)
	b1 := createBill(1)
	b2 := createBill(2)
	blockCounter := uint64(0)
	bills := map[string]*wallet.Bill{
		string(b1.Id): b1,
		string(b2.Id): b2,
	}

	var w *Wallet
	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getRoundNumber: func() (uint64, error) {
			return blockCounter, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b1, b2}), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{
				{Id: b1.GetID(), Value: b1.Value, TxHash: b1.TxHash},
				{Id: b2.GetID(), Value: b2.Value, TxHash: b2.TxHash},
			}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			var bill *wallet.Bill
			for _, tx := range recordedTransactions {
				if bytes.Equal(unitID, tx.UnitID()) {
					bill = bills[string(unitID)]
					if bill != nil {
						bill.TxHash = tx.Hash(crypto.SHA256)
					}
				}
			}
			if bill != nil {
				blockCounter++
				return createBlockProofResponseForDustTransfer(t, bill, nil, txTimeoutBlockCount, nil), nil
			} else {
				return nil, errors.New("bill not found")
			}
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w, _ = CreateTestWallet(t, backendMock)

	// test send successfully waits for confirmation
	_, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: pubKey, Amount: b1.Value + b2.Value}},
		WaitForConfirmation: true,
	})
	require.NoError(t, err)
}

func TestWalletSendFunction_ErrTxFailedToConfirm(t *testing.T) {
	pubKey := make([]byte, 33)
	b := createBill(1)

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
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b}), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			return nil, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w, _ := CreateTestWallet(t, backendMock)

	_, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: pubKey, Amount: b.Value}},
		WaitForConfirmation: true,
	})
	require.ErrorContains(t, err, "confirmation timeout")
}

func TestWholeBalanceIsSentUsingBillTransferOrder(t *testing.T) {
	// create wallet with single bill
	pubKey := make([]byte, 33)
	b := &wallet.Bill{
		Id:     []byte{1},
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
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b}), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			return nil, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w, _ := CreateTestWallet(t, backendMock)

	// when whole balance is spent
	_, err := w.Send(context.Background(), SendCmd{
		Receivers: []ReceiverData{{PubKey: pubKey, Amount: 100}},
	})
	require.NoError(t, err)

	// then bill transfer order should be sent
	require.Len(t, recordedTransactions, 1)
	btTx := parseBillTransferTx(t, recordedTransactions[0])
	require.EqualValues(t, 100, btTx.TargetValue)
}

func TestWalletSendFunction_LockedBillIsNotUsed(t *testing.T) {
	unitID := money.NewBillID(nil, []byte{123})
	w, _ := CreateTestWallet(t, withBackendMock(t, &backendMockReturnConf{
		balance:       70,
		billId:        unitID,
		billValue:     50,
		feeCreditBill: &wallet.Bill{Value: 1e8},
	}))
	pubKey, err := w.am.GetPublicKey(0)
	require.NoError(t, err)

	// lock the only bill in wallet
	err = w.unitLocker.LockUnit(unitlock.NewLockedUnit(
		pubKey,
		unitID,
		[]byte{1},
		unitlock.LockReasonCollectDust,
	))
	require.NoError(t, err)

	// test send returns error
	_, err = w.Send(context.Background(), SendCmd{
		Receivers: []ReceiverData{{PubKey: pubKey, Amount: 50}},
	})
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

func TestWalletSendFunction_BillWithExactAmount(t *testing.T) {
	// create test wallet with 2 bills with different values
	pubKey := make([]byte, 33)
	bills := []*wallet.Bill{{
		Id:     []byte{0},
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}, {
		Id:     []byte{1},
		Value:  77,
		TxHash: hash.Sum256([]byte{0x02}),
	}}

	var w *Wallet
	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return bills[0].Value + bills[1].Value, nil
		},
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse(bills), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: bills[0].GetID(), Value: bills[0].Value, TxHash: bills[0].TxHash}, {Id: bills[1].GetID(), Value: bills[1].Value, TxHash: bills[1].TxHash}}, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx := recordedTransactions[0]
			bills[1].TxHash = tx.Hash(crypto.SHA256)
			return createBlockProofResponseForDustTransfer(t, bills[1], nil, txTimeoutBlockCount, nil), nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w, _ = CreateTestWallet(t, backendMock)

	// run send command with amount equal to one of the bills
	_, err := w.Send(context.Background(), SendCmd{
		Receivers:           []ReceiverData{{PubKey: pubKey, Amount: bills[1].Value}},
		WaitForConfirmation: true,
	})

	// verify that the send command creates a single transfer for the bill with the exact value requested
	require.NoError(t, err)
	require.Len(t, recordedTransactions, 1)
	require.Equal(t, money.PayloadTypeTransfer, recordedTransactions[0].PayloadType())
	require.EqualValues(t, bills[1].Id, recordedTransactions[0].Payload.UnitID)
}

func TestWalletSendFunction_NWaySplit(t *testing.T) {
	// create test wallet with a single bill
	pubKey := make([]byte, 33)
	bills := []*wallet.Bill{{
		Id:     []byte{0},
		Value:  100,
		TxHash: hash.Sum256([]byte{0x01}),
	}}

	var w *Wallet
	var recordedTransactions []*types.TransactionOrder
	backendMock := &backendAPIMock{
		getBalance: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse(bills), nil
		},
		getBills: func(pubKey []byte) ([]*wallet.Bill, error) {
			var res []*wallet.Bill
			for _, b := range bills {
				res = append(res, &wallet.Bill{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash})
			}
			return res, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			return &wallet.Proof{}, nil
		},
		getFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
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

	// execute send command to multiple receivers
	_, err := w.Send(context.Background(), SendCmd{
		Receivers: []ReceiverData{
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
			{PubKey: pubKey, Amount: 5},
		},
		WaitForConfirmation: true,
	})

	// verify that the send command creates N-way split tx
	require.NoError(t, err)
	require.Len(t, recordedTransactions, 1)
	recordedTx := recordedTransactions[0]
	require.Equal(t, money.PayloadTypeSplit, recordedTx.PayloadType())
	require.EqualValues(t, bills[0].Id, recordedTx.Payload.UnitID)
	attr := &money.SplitAttributes{}
	err = recordedTx.UnmarshalAttributes(attr)
	require.NoError(t, err)
	require.Len(t, attr.TargetUnits, 5)
	for _, unit := range attr.TargetUnits {
		require.EqualValues(t, 5, unit.Amount)
		require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)), unit.OwnerCondition)
	}
}

func parseBillTransferTx(t *testing.T, tx *types.TransactionOrder) *money.TransferAttributes {
	transferTx := &money.TransferAttributes{}
	err := tx.UnmarshalAttributes(transferTx)
	require.NoError(t, err)
	return transferTx
}
