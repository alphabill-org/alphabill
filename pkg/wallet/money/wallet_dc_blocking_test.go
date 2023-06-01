package money

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/pkg/wallet"

	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestBlockingDcWithNormalBills(t *testing.T) {
	// wallet contains 2 normal bills
	bills := []*Bill{addBill(1), addBill(2)}
	nonce := calculateDcNonce(bills)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	billsList := createBillListResponse(bills)

	recordedTx := make(map[string]*txsystem.Transaction, 0)
	billListCallFlag := false
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 0, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			if billListCallFlag {
				return createBillListResponse([]*Bill{addBill(3)}), nil
			}
			billListCallFlag = true
			return billsList, nil
		},
		getProof: func(billId []byte) (*bp.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nil, 0, dcTimeoutBlockCount, nil), nil
				}
			}
			return nil, nil
		},
		fetchFeeCreditBill: func(ctx context.Context, unitID []byte) (*bp.Bill, error) {
			ac, _ := am.GetAccountKey(0)
			return &bp.Bill{
				Id:      ac.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &block.TxProof{},
			}, nil
		},
	}
	subBackendMock := &mockSubmitterBackend{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 0, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTx[string(tx.UnitId)] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTx[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{BlockNumber: 1, Tx: tx, Proof: nil}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)
	w.txBackend = subBackendMock

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and dc + swap txs should be sent
	require.Len(t, recordedTx, 3)
	for _, tx := range recordedTx {
		if tx.TransactionAttributes.TypeUrl == "type.googleapis.com/rpc.TransferDCAttributes" {
			dcTx := parseDcTx(t, tx)
			require.EqualValues(t, nonce, dcTx.Nonce)
		} else {
			swapTx := parseSwapTx(t, tx)
			require.EqualValues(t, 3, swapTx.TargetValue)
		}
	}

	// and expected swaps are cleared
	require.Empty(t, w.dcWg.swaps)
}

func TestBlockingDCWithDCBillsBeforeDCTimeout(t *testing.T) {
	t.SkipNow()
	// create wallet with 2 dc bills
	tempNonce := uint256.NewInt(1)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, tempNonce, util.Uint256ToBytes(tempNonce), 1, dcTimeoutBlockCount), addDcBill(t, k, tempNonce, util.Uint256ToBytes(tempNonce), 2, dcTimeoutBlockCount)}
	billsList := createBillListResponse(bills)

	recordedTx := make(map[string]*txsystem.Transaction, 0)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return 5, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*bp.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, nil, 0, dcTimeoutBlockCount, nil), nil
				}
			}
			return nil, nil
		},
		fetchFeeCreditBill: func(ctx context.Context, unitID []byte) (*bp.Bill, error) {
			ac, _ := am.GetAccountKey(0)
			return &bp.Bill{
				Id:      ac.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &block.TxProof{},
			}, nil
		},
	}
	subBackendMock := &mockSubmitterBackend{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return 5, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTx[string(tx.UnitId)] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTx[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{BlockNumber: 1, Tx: tx, Proof: nil}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)
	w.txBackend = subBackendMock

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and swap tx should be sent
	for _, tx := range recordedTx {
		if tx.TransactionAttributes.TypeUrl == "type.googleapis.com/rpc.SwapDCAttributes" {
			swapTx := parseSwapTx(t, tx)
			require.EqualValues(t, 3, swapTx.TargetValue)
		}
	}

	// and expected swaps are cleared
	require.Empty(t, w.dcWg.swaps)
}

func TestBlockingDCWithExistingExpiredDCBills(t *testing.T) {
	// create wallet with 2 timed out dc bills
	tempNonce := uint256.NewInt(1)
	am, err := account.NewManager(t.TempDir(), "", true)
	require.NoError(t, err)
	_ = am.CreateKeys("")
	k, _ := am.GetAccountKey(0)
	bills := []*Bill{addDcBill(t, k, uint256.NewInt(1), util.Uint256ToBytes(tempNonce), 1, 0), addDcBill(t, k, uint256.NewInt(2), util.Uint256ToBytes(tempNonce), 2, 0)}
	billsList := createBillListResponse(bills)

	recordedTx := make(map[string]*txsystem.Transaction, 0)
	backendMock := &backendAPIMock{
		getRoundNumber: func() (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		listBills: func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error) {
			return billsList, nil
		},
		getProof: func(billId []byte) (*bp.Bills, error) {
			for _, b := range bills {
				if bytes.Equal(util.Uint256ToBytes(b.Id), billId) {
					return createBlockProofResponse(t, b, util.Uint256ToBytes(tempNonce), 0, dcTimeoutBlockCount, nil), nil
				}
			}
			return nil, nil
		},
		fetchFeeCreditBill: func(ctx context.Context, unitID []byte) (*bp.Bill, error) {
			ac, _ := am.GetAccountKey(0)
			return &bp.Bill{
				Id:      ac.PrivKeyHash,
				Value:   100 * 1e8,
				TxProof: &block.TxProof{},
			}, nil
		},
	}
	subBackendMock := &mockSubmitterBackend{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			return dcTimeoutBlockCount, nil
		},
		postTransactions: func(ctx context.Context, pubKey wallet.PubKey, txs *txsystem.Transactions) error {
			for _, tx := range txs.Transactions {
				recordedTx[string(tx.UnitId)] = tx
			}
			return nil
		},
		getTxProof: func(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
			tx, found := recordedTx[string(unitID)]
			if !found {
				return nil, errors.New("tx not found")
			}
			return &wallet.Proof{BlockNumber: 1, Tx: tx, Proof: nil}, nil
		},
	}

	w, _ := CreateTestWalletWithManager(t, backendMock, am)
	w.txBackend = subBackendMock

	// when blocking dust collector runs
	_ = runBlockingDc(t, w)

	// wait for confirmation
	waitForExpectedSwap(w)

	// and swap tx should be sent
	for _, tx := range recordedTx {
		if tx.TransactionAttributes.TypeUrl == "type.googleapis.com/rpc.SwapDCAttributes" {
			swapTx := parseSwapTx(t, tx)
			require.EqualValues(t, 3, swapTx.TargetValue)
		}
	}

	// and expected swaps are cleared
	require.Empty(t, w.dcWg.swaps)
}

func runBlockingDc(t *testing.T, w *Wallet) *sync.WaitGroup {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := w.collectDust(context.Background(), true, 0)
		require.NoError(t, err)
		wg.Done()
	}()
	return &wg
}

func waitForExpectedSwap(w *Wallet) {
	waitForCondition(func() bool {
		w.dcWg.mu.Lock()
		defer w.dcWg.mu.Unlock()
		return len(w.dcWg.swaps) > 0
	})
}

func waitForCondition(waitCondition func() bool) {
	t1 := time.Now()
	for {
		ok := waitCondition()
		if ok {
			break
		}
		if time.Since(t1).Seconds() > 5 {
			log.Warning("breaking on 5s timeout")
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
