package money

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestDcJobWithExistingDcBills(t *testing.T) {
	// wallet contains 2 dc bills with the same nonce that have timed out
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{
		addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(uint256.NewInt(1)), 1, dcTimeoutBlockCount),
		addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(uint256.NewInt(1)), 2, dcTimeoutBlockCount),
	}
	nonce := calculateDcNonce(bills)
	billsList := createBillListResponse(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nonce, 0, dcTimeoutBlockCount, nil), nil
				}
			}
			return nil, nil
		},
		fetchFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then swap tx is broadcast
	require.Len(t, recordedTxs, 1)
	var tx *types.TransactionOrder
	var txSwap *money.SwapDCAttributes
	for _, recTx := range recordedTxs {
		if recTx.PayloadType() == "swapDC" {
			txSwap = parseSwapTx(t, recTx)
			tx = recTx
		}
	}

	// and verify each dc tx id = nonce = swap.id
	require.Len(t, txSwap.DcTransfers, 2)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTx(t, txSwap.DcTransfers[i].TransactionOrder)
		require.EqualValues(t, nonce, dcTx.Nonce)
		require.EqualValues(t, nonce, tx.UnitID())
	}
}

func TestDcJobWithExistingDcAndNonDcBills(t *testing.T) {
	// wallet contains timed out dc bill and normal bill
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bill := addBill(1)
	dc := addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(uint256.NewInt(1)), 2, dcTimeoutBlockCount)
	billList := []*Bill{bill, dc}
	nonce := calculateDcNonce(billList)
	billsList := createBillListResponse(billList)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range billList {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nonce, 0, dcTimeoutBlockCount, nil), nil
				}
			}
			return nil, nil
		},
		fetchFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then swap tx is sent for the timed out dc bill
	require.Len(t, recordedTxs, 1)
	var tx *types.TransactionOrder
	var txSwap *money.SwapDCAttributes
	for _, recTx := range recordedTxs {
		if recTx.PayloadType() == "swapDC" {
			txSwap = parseSwapTx(t, recTx)
			tx = recTx
		}
	}

	// and verify nonce = swap.id = dc tx id
	require.Len(t, txSwap.DcTransfers, 1)
	for i := 0; i < len(txSwap.DcTransfers); i++ {
		dcTx := parseDcTx(t, txSwap.DcTransfers[i].TransactionOrder)
		require.EqualValues(t, nonce, dcTx.Nonce)
		require.EqualValues(t, nonce, tx.UnitID())
	}
}

func TestDcJobWithExistingNonDcBills(t *testing.T) {
	// wallet contains 2 non dc bills
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	bills := []*Bill{addBill(1), addBill(2)}
	nonce := calculateDcNonce(bills)
	billsList := createBillListResponse(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	recordedNonces := make([][]byte, 0)
	billListCallFlag := false
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			if billListCallFlag {
				return createBillListResponse([]*Bill{addBill(uint64(3))}), nil
			}
			billListCallFlag = true
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nonce, 0, dcTimeoutBlockCount, nil), nil
				}
			}
			return nil, nil
		},
		fetchFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
				if tx.PayloadType() == "transDC" {
					recordedNonces = append(recordedNonces, parseDcTx(t, tx).Nonce)
				}
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then dust txs are broadcast (plus swap)
	require.Len(t, recordedTxs, 3)

	// and nonces are equal
	require.Len(t, recordedNonces, 2)
	require.EqualValues(t, recordedNonces[0], recordedNonces[1])
}

func TestDcJobSendsSwapsIfDcBillTimeoutHasBeenReached(t *testing.T) {
	// wallet contains 2 dc bills that both have timed out
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(uint256.NewInt(1)), 1, dcTimeoutBlockCount), addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(uint256.NewInt(1)), 2, dcTimeoutBlockCount)}
	nonce := calculateDcNonce(bills)
	billsList := createBillListResponse(bills)

	recordedTxs := make(map[string]*types.TransactionOrder, 0)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*wallet.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nonce, 0, dcTimeoutBlockCount, nil), nil
				}
			}
			return nil, nil
		},
		fetchFeeCreditBill: func(ctx context.Context, unitID []byte) (*wallet.Bill, error) {
			k, _ := am.GetAccountKey(0)
			return &wallet.Bill{
				Id:      k.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &wallet.Proof{},
			}, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTxs[string(tx.UnitID())] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTxs[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{TxRecord: &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)

	// when dust collector runs
	err = w.collectDust(context.Background(), false, 0)
	require.NoError(t, err)

	// then 1 swap txs must be broadcast
	require.Len(t, recordedTxs, 1)
	for _, tx := range recordedTxs {
		require.NotNil(t, parseSwapTx(t, tx))
	}
}
