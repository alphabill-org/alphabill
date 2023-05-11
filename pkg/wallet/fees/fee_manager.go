package fees

import (
	"context"
	"errors"
	"sort"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
)

const (
	maxFee              = uint64(1)
	txTimeoutBlockCount = 10
)

type (
	TxPublisher interface {
		SendTx(ctx context.Context, tx *txsystem.Transaction, senderPubKey []byte) (*block.TxProof, error)
	}

	PartitionDataProvider interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		FetchFeeCreditBill(ctx context.Context, unitID []byte) (*bp.Bill, error)
	}

	MoneyClient interface {
		GetBills(pubKey []byte) ([]*bp.Bill, error)
		PartitionDataProvider
	}

	FeeManager struct {
		am account.Manager

		// money partition fields
		moneySystemID      []byte
		moneyTxPublisher   TxPublisher
		moneyBackendClient MoneyClient

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
	moneyBackendClient MoneyClient,
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
// Returns transferFC and addFC transaction proofs.
func (w *FeeManager) AddFeeCredit(ctx context.Context, cmd AddFeeCmd) ([]*block.TxProof, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	// fetch bills
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	bills, err := w.moneyBackendClient.GetBills(accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	// sort bills by value in descending order
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	if len(bills) == 0 {
		return nil, errors.New("wallet does not contain any bills")
	}
	// verify bill is large enough for required amount
	billToTransfer := bills[0]
	if billToTransfer.Value < cmd.Amount+maxFee {
		return nil, errors.New("wallet does not have a bill large enough for fee transfer")
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
	transferFCProof, err := w.moneyTxPublisher.SendTx(ctx, tx, accountKey.PubKey)
	if err != nil {
		return nil, err
	}

	// send addFC to user partition
	log.Info("sending add fee credit transaction")
	addFCTx, err := txbuilder.CreateAddFCTx(accountKey.PrivKeyHash, transferFCProof, accountKey, w.userPartitionSystemID, userPartitionTimeout)
	if err != nil {
		return nil, err
	}
	addFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, addFCTx, accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	return []*block.TxProof{transferFCProof, addFCProof}, err
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns closeFC and reclaimFC transaction proofs.
func (w *FeeManager) ReclaimFeeCredit(ctx context.Context, cmd ReclaimFeeCmd) ([]*block.TxProof, error) {
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
		return nil, errors.New("insufficient fee credit balance for transaction(s)")
	}

	// fetch bills
	bills, err := w.moneyBackendClient.GetBills(k.PubKey)
	if err != nil {
		return nil, err
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
	closeFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, tx, k.PubKey)
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
	reclaimFCTx, err := txbuilder.CreateReclaimFCTx(w.moneySystemID, targetBill.GetId(), moneyTimeout, closeFCProof, targetBill.TxHash, k)
	if err != nil {
		return nil, err
	}
	reclaimFCProof, err := w.moneyTxPublisher.SendTx(ctx, reclaimFCTx, k.PubKey)
	if err != nil {
		return nil, err
	}
	return []*block.TxProof{closeFCProof, reclaimFCProof}, nil
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

func (c *AddFeeCmd) isValid() error {
	if c.Amount == 0 {
		return errors.New("fee credit amount must be positive")
	}
	return nil
}
