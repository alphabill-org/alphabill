package testutil

import (
	"context"
	"errors"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
)

type (
	BackendMockReturnConf struct {
		Balance                  uint64
		RoundNumber              uint64
		BillID                   types.UnitID
		BillValue                uint64
		BillTxHash               string
		ProofList                []string
		CustomBillList           string
		CustomPath               string
		CustomFullPath           string
		CustomResponse           string
		FeeCreditBill            *wallet.Bill
		PostTransactionsResponse interface{}
	}
)

type BackendAPIMock struct {
	GetBalanceFn       func(pubKey []byte, includeDCBills bool) (uint64, error)
	ListBillsFn        func(pubKey []byte, includeDCBills bool) (*backend.ListBillsResponse, error)
	GetBillsFn         func(pubKey []byte) ([]*wallet.Bill, error)
	GetRoundNumberFn   func() (uint64, error)
	GetTxProofFn       func(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	GetFeeCreditBillFn func(ctx context.Context, unitID []byte) (*wallet.Bill, error)
	PostTransactionsFn func(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
}

func (b *BackendAPIMock) GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error) {
	if b.GetBillsFn != nil {
		return b.GetBillsFn(pubKey)
	}
	return nil, errors.New("GetBillsFn not implemented")
}

func (b *BackendAPIMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if b.GetRoundNumberFn != nil {
		return b.GetRoundNumberFn()
	}
	return 0, errors.New("getRoundNumber not implemented")
}

func (b *BackendAPIMock) GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error) {
	if b.GetFeeCreditBillFn != nil {
		return b.GetFeeCreditBillFn(ctx, unitID)
	}
	return nil, errors.New("getFeeCreditBill not implemented")
}

func (b *BackendAPIMock) GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error) {
	if b.GetBalanceFn != nil {
		return b.GetBalanceFn(pubKey, includeDCBills)
	}
	return 0, errors.New("getBalance not implemented")
}

func (b *BackendAPIMock) ListBills(ctx context.Context, pubKey []byte, includeDCBills bool, offsetKey string, limit int) (*backend.ListBillsResponse, error) {
	if b.ListBillsFn != nil {
		return b.ListBillsFn(pubKey, includeDCBills)
	}
	return nil, errors.New("listBills not implemented")
}

func (b *BackendAPIMock) GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error) {
	if b.GetTxProofFn != nil {
		return b.GetTxProofFn(ctx, unitID, txHash)
	}
	return nil, errors.New("getTxProof not implemented")
}

func (b *BackendAPIMock) PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error {
	if b.PostTransactionsFn != nil {
		return b.PostTransactionsFn(ctx, pubKey, txs)
	}
	return nil
}
