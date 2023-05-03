package fees

import (
	"context"
	"errors"
	"sort"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
)

const (
	maxFee              = uint64(1)
	txTimeoutBlockCount = 10
)

var (
	ErrInvalidCreateFeeCreditAmount = errors.New("fee credit amount must be positive")
	ErrInsufficientBillValue        = errors.New("wallet does not have a bill large enough for fee transfer")
	ErrAddFCInsufficientBalance     = errors.New("insufficient balance for transaction and transaction fee")
	ErrInsufficientFeeCredit        = errors.New("insufficient fee credit balance for transaction(s)")
)

type (
	TxPublisher interface {
		SendTx(ctx context.Context, tx *txsystem.Transaction, senderPubKey []byte) (*block.TxProof, error)
	}

	PartitionDataProvider interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		FetchFeeCreditBill(ctx context.Context, unitID []byte) (*bp.Bill, error)
	}

	FeeManager struct {
		am account.Manager

		// money partition fields
		moneySystemID      []byte
		moneyTxPublisher   TxPublisher
		moneyBackendClient *client.MoneyBackendClient

		// user partition fields
		userPartitionSystemID      []byte
		userPartitionTxPublisher   TxPublisher
		userPartitionBackendClient PartitionDataProvider
	}

	GetFeeCreditCmd struct {
		AccountIndex uint64
	}

	AddFeeCmd struct {
		AccountIndex uint64
		Amount       uint64
	}

	ReclaimFeeCmd struct {
		AccountIndex uint64
	}
)

// NewFeeManager creates new fee credit manager.
// Parameters:
// - account manager
//
// - money partition:
//   - systemID
//   - tx publisher with proof confirmation
//   - money backend client
//
// - user partition:
//   - systemID
//   - tx publisher with proof confirmation
//   - partition data provider e.g. backend client
func NewFeeManager(
	am account.Manager,
	moneySystemID []byte,
	moneyTxPublisher TxPublisher,
	moneyBackendClient *client.MoneyBackendClient,
	partitionSystemID []byte,
	partitionTxPublisher TxPublisher,
	partitionBackendClient PartitionDataProvider,
) *FeeManager {
	return &FeeManager{
		am:                         am,
		moneySystemID:              moneySystemID,
		moneyTxPublisher:           moneyTxPublisher,
		moneyBackendClient:         moneyBackendClient,
		userPartitionSystemID:      partitionSystemID,
		userPartitionTxPublisher:   partitionTxPublisher,
		userPartitionBackendClient: partitionBackendClient,
	}
}

// AddFeeCredit creates fee credit for the given amount.
// Wallet must have a bill large enough for the required amount plus fees.
// Returns "add fee credit" transaction proof.
func (w *FeeManager) AddFeeCredit(ctx context.Context, cmd AddFeeCmd) (*block.TxProof, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}
	balance, err := w.getMoneyBalance(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	// must have enough balance for two txs (transferFC + addFC) and not end up with zero sum
	maxTotalFees := 2 * maxFee
	if cmd.Amount+maxTotalFees > balance {
		return nil, ErrAddFCInsufficientBalance
	}

	// fetch bills
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	listBillsResponse, err := w.moneyBackendClient.ListBills(accountKey.PubKey, false)
	if err != nil {
		return nil, err
	}
	// convert list view bills to []*bp.Bill
	var bills []*bp.Bill
	for _, b := range listBillsResponse.Bills {
		bills = append(bills, newBillFromVM(b))
	}
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})

	// verify bill is large enough for required amount
	billToTransfer := bills[0]
	if billToTransfer.Value < cmd.Amount+maxTotalFees {
		return nil, ErrInsufficientBillValue
	}

	// fetch fee credit bill
	fcb, err := w.GetFeeCreditBill(ctx, GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}

	// fetch user partition round number for timeouts
	userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	userPartitionTimeout := userPartitionRoundNumber + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	// fetch money round number for timeouts
	moneyRoundNumber, err := w.moneyBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	moneyTimeout := moneyRoundNumber + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	// send transferFC to money partition
	log.Info("sending transfer fee credit transaction")
	tx, err := txbuilder.CreateTransferFCTx(cmd.Amount, accountKey.PrivKeyHash, fcb.GetTxHash(), accountKey, w.moneySystemID, w.userPartitionSystemID, billToTransfer, moneyTimeout, userPartitionRoundNumber, userPartitionTimeout)
	if err != nil {
		return nil, err
	}
	txProof, err := w.moneyTxPublisher.SendTx(ctx, tx, accountKey.PubKey)
	if err != nil {
		return nil, err
	}

	// send addFC to user partition
	log.Info("sending add fee credit transaction")
	addFCTx, err := txbuilder.CreateAddFCTx(accountKey.PrivKeyHash, txProof, accountKey, w.userPartitionSystemID, userPartitionTimeout)
	if err != nil {
		return nil, err
	}
	return w.userPartitionTxPublisher.SendTx(ctx, addFCTx, accountKey.PubKey)
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns "reclaim fee credit" transaction proof.
func (w *FeeManager) ReclaimFeeCredit(ctx context.Context, cmd ReclaimFeeCmd) (*block.TxProof, error) {
	k, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	// fetch user partition timeout
	userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	userPartitionTimeout := userPartitionRoundNumber + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	// fetch fee credit bill
	fcb, err := w.GetFeeCreditBill(ctx, GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}
	if fcb.GetValue() == 0 {
		return nil, ErrInsufficientFeeCredit
	}

	// fetch bills
	listBillsResponse, err := w.moneyBackendClient.ListBills(k.PubKey, false)
	if err != nil {
		return nil, err
	}

	// convert list view bills to []*bp.Bill
	var bills []*bp.Bill
	for _, b := range listBillsResponse.Bills {
		bills = append(bills, newBillFromVM(b))
	}

	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	if len(bills) == 0 {
		return nil, errors.New("wallet must have a source bill to which to add reclaimed fee credits")
	}
	targetBill := bills[0]

	// send closeFC tx to user partition
	log.Info("sending close fee credit transaction")
	tx, err := txbuilder.CreateCloseFCTx(w.userPartitionSystemID, fcb.GetId(), userPartitionTimeout, fcb.Value, targetBill.GetId(), targetBill.TxHash, k)
	if err != nil {
		return nil, err
	}
	txProof, err := w.userPartitionTxPublisher.SendTx(ctx, tx, k.PubKey)
	if err != nil {
		return nil, err
	}

	// fetch money partition timeout
	moneyRoundNumber, err := w.moneyBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	moneyTimeout := moneyRoundNumber + txTimeoutBlockCount
	if err != nil {
		return nil, err
	}

	// send reclaimFC tx to money partition
	log.Info("sending reclaim fee credit transaction")
	reclaimFCTx, err := txbuilder.CreateReclaimFCTx(w.moneySystemID, targetBill.GetId(), moneyTimeout, txProof, targetBill.TxHash, k)
	if err != nil {
		return nil, err
	}
	return w.moneyTxPublisher.SendTx(ctx, reclaimFCTx, k.PubKey)
}

// GetFeeCreditBill returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *FeeManager) GetFeeCreditBill(ctx context.Context, cmd GetFeeCreditCmd) (*bp.Bill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.userPartitionBackendClient.FetchFeeCreditBill(ctx, accountKey.PrivKeyHash)
}

func (w *FeeManager) getMoneyBalance(accountIndex uint64) (uint64, error) {
	pubkey, err := w.am.GetPublicKey(accountIndex)
	if err != nil {
		return 0, err
	}
	return w.moneyBackendClient.GetBalance(pubkey, false)
}

func (c *AddFeeCmd) isValid() error {
	if c.Amount == 0 {
		return ErrInvalidCreateFeeCreditAmount
	}
	return nil
}

// newBillFromVM converts ListBillVM to Bill structs
func newBillFromVM(b *backend.ListBillVM) *bp.Bill {
	return &bp.Bill{
		Id:       b.Id,
		Value:    b.Value,
		TxHash:   b.TxHash,
		IsDcBill: b.IsDCBill,
	}
}
