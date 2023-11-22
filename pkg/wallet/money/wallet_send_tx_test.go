package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	beclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/testutil"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

func TestWalletSendFunction_Ok(t *testing.T) {
	w := createTestWallet(t, withBackendMock(t, &testutil.BackendMockReturnConf{
		Balance:   70,
		BillID:    money.NewBillID(nil, []byte{0}),
		BillValue: 50,
		FeeCreditBill: &wallet.Bill{
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
	w := createTestWallet(t, withBackendMock(t, &testutil.BackendMockReturnConf{}))
	invalidPubKey := make([]byte, 32)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInvalidPubKey
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: invalidPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "invalid public key: public key must be in compressed secp256k1 format: got 32 "+
		"bytes, expected 33 bytes for public key 0x0000000000000000000000000000000000000000000000000000000000000000")
}

func TestWalletSendFunction_InsufficientBalance(t *testing.T) {
	w := createTestWallet(t, withBackendMock(t, &testutil.BackendMockReturnConf{Balance: 10}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)
	ctx := context.Background()

	// test ErrInsufficientBalance
	_, err := w.Send(ctx, SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "insufficient balance for transaction")
}

func TestWalletSendFunction_ClientError(t *testing.T) {
	w := createTestWallet(t, withBackendMock(t, &testutil.BackendMockReturnConf{
		Balance:   70,
		BillID:    money.NewBillID(nil, []byte{0}),
		BillValue: 50,
		FeeCreditBill: &wallet.Bill{
			Id:    []byte{},
			Value: 100 * 1e8,
		},
		PostTransactionsResponse: map[string]string{"message": "some error"},
	}))
	validPubKey := make([]byte, 33)
	amount := uint64(50)

	// test PostTransactions returns error
	_, err := w.Send(context.Background(), SendCmd{Receivers: []ReceiverData{{PubKey: validPubKey, Amount: amount}}})
	require.ErrorContains(t, err, "failed to send transactions: backend responded 500 Internal Server Error: some error")
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
	backendMock := &testutil.BackendAPIMock{
		GetBalanceFn: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		GetRoundNumberFn: func() (uint64, error) {
			return 0, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b}), nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx := recordedTransactions[0]
			b.TxHash = tx.Hash(crypto.SHA256)
			return createBlockProofResponseForDustTransfer(t, b, nil, txTimeoutBlockCount, nil), nil
		},
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
			}, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w = createTestWallet(t, backendMock)

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

	var recordedTransactions []*types.TransactionOrder
	backendMock := &testutil.BackendAPIMock{
		GetBalanceFn: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		GetRoundNumberFn: func() (uint64, error) {
			return 0, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b1, b2}), nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{
				{Id: b1.GetID(), Value: b1.Value, TxHash: b1.TxHash},
				{Id: b2.GetID(), Value: b2.Value, TxHash: b2.TxHash},
			}, nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
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
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8}, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w := createTestWallet(t, backendMock)

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
	backendMock := &testutil.BackendAPIMock{
		GetBalanceFn: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		GetRoundNumberFn: func() (uint64, error) {
			return blockCounter, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b1, b2}), nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{
				{Id: b1.GetID(), Value: b1.Value, TxHash: b1.TxHash},
				{Id: b2.GetID(), Value: b2.Value, TxHash: b2.TxHash},
			}, nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
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
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
			}, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w = createTestWallet(t, backendMock)

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
	backendMock := &testutil.BackendAPIMock{
		GetBalanceFn: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		GetRoundNumberFn: func() (uint64, error) {
			if len(recordedTransactions) == 0 {
				return 0, nil
			}
			return 2 * txTimeoutBlockCount, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b}), nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8}, nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			return nil, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w := createTestWallet(t, backendMock)

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
	backendMock := &testutil.BackendAPIMock{
		GetBalanceFn: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		GetRoundNumberFn: func() (uint64, error) {
			return 0, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse([]*wallet.Bill{b}), nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}}, nil
		},
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			return &wallet.Bill{Id: []byte{}, Value: 100 * 1e8}, nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			return nil, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w := createTestWallet(t, backendMock)

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
	w := createTestWallet(t, withBackendMock(t, &testutil.BackendMockReturnConf{
		Balance:       70,
		BillID:        unitID,
		BillValue:     50,
		FeeCreditBill: &wallet.Bill{Value: 1e8},
	}))
	pubKey, err := w.am.GetPublicKey(0)
	require.NoError(t, err)

	// lock the only bill in wallet
	err = w.unitLocker.LockUnit(unitlock.NewLockedUnit(
		pubKey,
		unitID,
		[]byte{1},
		money.DefaultSystemIdentifier,
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
	backendMock := &testutil.BackendAPIMock{
		GetBalanceFn: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return bills[0].Value + bills[1].Value, nil
		},
		GetRoundNumberFn: func() (uint64, error) {
			return 0, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse(bills), nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			return []*wallet.Bill{{Id: bills[0].GetID(), Value: bills[0].Value, TxHash: bills[0].TxHash}, {Id: bills[1].GetID(), Value: bills[1].Value, TxHash: bills[1].TxHash}}, nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx := recordedTransactions[0]
			bills[1].TxHash = tx.Hash(crypto.SHA256)
			return createBlockProofResponseForDustTransfer(t, bills[1], nil, txTimeoutBlockCount, nil), nil
		},
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
			}, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w = createTestWallet(t, backendMock)

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
	backendMock := &testutil.BackendAPIMock{
		GetBalanceFn: func(pubKey []byte, includeDCBills bool) (uint64, error) {
			return 100, nil
		},
		GetRoundNumberFn: func() (uint64, error) {
			return 0, nil
		},
		ListBillsFn: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return createBillListResponse(bills), nil
		},
		GetBillsFn: func(pubKey []byte) ([]*wallet.Bill, error) {
			var res []*wallet.Bill
			for _, b := range bills {
				res = append(res, &wallet.Bill{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash})
			}
			return res, nil
		},
		GetTxProofFn: func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			return &wallet.Proof{}, nil
		},
		GetFeeCreditBillFn: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			ac, _ := w.am.GetAccountKey(0)
			return &wallet.Bill{
				Id:    money.NewFeeCreditRecordID(nil, ac.PubKeyHash.Sha256),
				Value: 100 * 1e8,
			}, nil
		},
		PostTransactionsFn: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			recordedTransactions = append(recordedTransactions, txs.Transactions...)
			return nil
		},
	}
	w = createTestWallet(t, backendMock)

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
		require.EqualValues(t, templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)), unit.OwnerCondition)
	}
}

func createBlockProofResponseForDustTransfer(t *testing.T, b *wallet.Bill, targetBill *wallet.Bill, timeout uint64, k *account.AccountKey) *wallet.Proof {
	w := createTestWallet(t, nil)
	if k == nil {
		var err error
		k, err = w.GetAccountManager().GetAccountKey(0)
		require.NoError(t, err)
	}
	if targetBill == nil {
		targetBill = &wallet.Bill{Id: []byte{0}, TxHash: []byte{}}
	}
	dcTx, err := txbuilder.NewDustTx(k, w.SystemID(), b, targetBill.Id, targetBill.TxHash, timeout)
	require.NoError(t, err)

	txRecord := &types.TransactionRecord{TransactionOrder: dcTx}
	txProof := &wallet.Proof{
		TxRecord: txRecord,
		TxProof: &types.TxProof{
			BlockHeaderHash:    []byte{0},
			Chain:              []*types.GenericChainItem{{Hash: []byte{0}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: timeout}},
		},
	}
	return txProof
}

func parseBillTransferTx(t *testing.T, tx *types.TransactionOrder) *money.TransferAttributes {
	transferTx := &money.TransferAttributes{}
	err := tx.UnmarshalAttributes(transferTx)
	require.NoError(t, err)
	return transferTx
}

func createTestWallet(t *testing.T, backend BackendAPI) *Wallet {
	dir := t.TempDir()
	am, err := account.NewManager(dir, "", true)
	require.NoError(t, err)

	unitLocker, err := unitlock.NewUnitLocker(dir)
	require.NoError(t, err)

	feeManagerDB, err := fees.NewFeeManagerDB(dir)
	require.NoError(t, err)

	return createTestWalletWithManagerAndUnitLocker(t, backend, am, feeManagerDB, unitLocker)
}

func createTestWalletWithManagerAndUnitLocker(t *testing.T, backend BackendAPI, am account.Manager, feeManagerDB fees.FeeManagerDB, unitLocker *unitlock.UnitLocker) *Wallet {
	err := CreateNewWallet(am, "")
	require.NoError(t, err)

	w, err := LoadExistingWallet(am, unitLocker, feeManagerDB, backend, logger.New(t))
	require.NoError(t, err)

	return w
}

func withBackendMock(t *testing.T, br *testutil.BackendMockReturnConf) BackendAPI {
	_, serverAddr := MockBackendCalls(br)
	restClient, err := beclient.New(serverAddr.Host)
	require.NoError(t, err)
	return restClient
}

func createBill(value uint64) *wallet.Bill {
	return &wallet.Bill{
		Id:     util.Uint64ToBytes32(value),
		Value:  value,
		TxHash: hash.Sum256([]byte{byte(value)}),
	}
}

func createBillListResponse(bills []*wallet.Bill) *backend.ListBillsResponse {
	billVMs := make([]*wallet.Bill, len(bills))
	for i, b := range bills {
		billVMs[i] = &wallet.Bill{
			Id:                   b.GetID(),
			Value:                b.Value,
			TxHash:               b.TxHash,
			DCTargetUnitID:       b.DCTargetUnitID,
			DCTargetUnitBacklink: b.DCTargetUnitBacklink,
		}
	}
	return &backend.ListBillsResponse{Bills: billVMs}
}
