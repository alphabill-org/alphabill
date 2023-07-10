package fees

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
)

const (
	maxFee                        = uint64(1)
	txTimeoutBlockCount           = 10
	xPartitionTxTimeoutBlockCount = 60 // one minute to do x-partition tx
)

type (
	TxPublisher interface {
		SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error)
		Close()
	}

	PartitionDataProvider interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(ctx context.Context, unitID wallet.UnitID) (*wallet.Bill, error)
		GetTxProof(ctx context.Context, unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
		GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error)
	}

	MoneyClient interface {
		GetBills(pubKey []byte) ([]*wallet.Bill, error)
		GetLockedFeeCredit(ctx context.Context, systemID []byte, fcbID []byte) (*types.TransactionRecord, error)
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

	AddFeeCmdResponse struct {
		TransferFC *wallet.Proof
		AddFC      *wallet.Proof
	}

	ReclaimFeeCmdResponse struct {
		CloseFC   *wallet.Proof
		ReclaimFC *wallet.Proof
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
// Returns transferFC and/or addFC transaction proofs.
func (w *FeeManager) AddFeeCredit(ctx context.Context, cmd AddFeeCmd) (*AddFeeCmdResponse, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}

	// fetch fee credit bill
	fcb, err := w.GetFeeCredit(ctx, GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, err
	}

	// fetch user partition round number for timeouts
	userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	userPartitionTimeout := userPartitionRoundNumber + xPartitionTxTimeoutBlockCount

	// fetch existing locked credit or create new transfer
	res := &AddFeeCmdResponse{}
	transferFCProof, err := w.getUnaddedTransferFCProof(ctx, accountKey, fcb, userPartitionRoundNumber, cmd.Amount)
	if err != nil {
		return nil, err
	}
	if transferFCProof == nil {
		transferFCProof, err = w.sendTransferFC(ctx, cmd, accountKey, fcb, userPartitionRoundNumber, userPartitionTimeout)
		if err != nil {
			return nil, err
		}
		res.TransferFC = transferFCProof // add transferFC proof only if we create new transferFC
	}

	// send addFC to user partition
	log.Info("sending add fee credit transaction")
	addFCTx, err := txbuilder.NewAddFCTx(accountKey.PrivKeyHash, transferFCProof, accountKey, w.userPartitionSystemID, userPartitionTimeout)
	if err != nil {
		return nil, err
	}
	addFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, addFCTx, accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	res.AddFC = addFCProof
	return res, nil
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns closeFC and/or reclaimFC transaction proofs.
func (w *FeeManager) ReclaimFeeCredit(ctx context.Context, cmd ReclaimFeeCmd) (*ReclaimFeeCmdResponse, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	bills, err := w.getSortedBills(accountKey)
	if err != nil {
		return nil, err
	}

	var targetBill *wallet.Bill
	res := &ReclaimFeeCmdResponse{}

	// fetch existing closed credit or create new closeFC tx
	closeFCProof, targetBill, err := w.getUnreclaimedCloseFCProof(ctx, accountKey, bills)
	if err != nil {
		return nil, err
	}
	if closeFCProof == nil {
		closeFCProof, targetBill, err = w.sendCloseFC(ctx, accountKey, bills)
		if err != nil {
			return nil, err
		}
		res.CloseFC = closeFCProof // add closeFC proof only if we create new closeFC
	}

	reclaimFCProof, err := w.sendReclaimFCTx(ctx, closeFCProof, targetBill, accountKey)
	if err != nil {
		return nil, err
	}
	res.ReclaimFC = reclaimFCProof
	return res, nil
}

// GetFeeCredit returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *FeeManager) GetFeeCredit(ctx context.Context, cmd GetFeeCreditCmd) (*wallet.Bill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	return w.userPartitionBackendClient.GetFeeCreditBill(ctx, accountKey.PrivKeyHash)
}

func (w *FeeManager) Close() {
	w.moneyTxPublisher.Close()
	w.userPartitionTxPublisher.Close()
}

func (w *FeeManager) sendTransferFC(ctx context.Context, cmd AddFeeCmd, accountKey *account.AccountKey, fcb *wallet.Bill, userPartitionRoundNumber uint64, userPartitionTimeout uint64) (*wallet.Proof, error) {
	bills, err := w.getSortedBills(accountKey)
	if err != nil {
		return nil, err
	}
	if len(bills) == 0 {
		return nil, errors.New("wallet does not contain any bills")
	}
	// verify bill is large enough for required amount
	billToTransfer := bills[0]
	if billToTransfer.Value < cmd.Amount+maxFee {
		return nil, errors.New("wallet does not have a bill large enough for fee transfer")
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
	tx, err := txbuilder.NewTransferFCTx(cmd.Amount, accountKey.PrivKeyHash, fcb.GetLastAddFCTxHash(), accountKey, w.moneySystemID, w.userPartitionSystemID, billToTransfer, moneyTimeout, userPartitionRoundNumber, userPartitionTimeout)
	if err != nil {
		return nil, err
	}
	return w.moneyTxPublisher.SendTx(ctx, tx, accountKey.PubKey)
}

func (w *FeeManager) sendCloseFC(ctx context.Context, k *account.AccountKey, bills []*wallet.Bill) (*wallet.Proof, *wallet.Bill, error) {
	if len(bills) == 0 {
		return nil, nil, errors.New("wallet must have a source bill to which to add reclaimed fee credits")
	}
	targetBill := bills[0]
	fcb, err := w.userPartitionBackendClient.GetFeeCreditBill(ctx, k.PrivKeyHash)
	if err != nil {
		return nil, nil, err
	}
	if fcb.GetValue() == 0 {
		return nil, nil, errors.New("insufficient fee credit balance for transaction(s)")
	}
	// fetch user partition timeout
	userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, nil, err
	}
	userPartitionTimeout := userPartitionRoundNumber + xPartitionTxTimeoutBlockCount

	// send closeFC tx to user partition
	log.Info("sending close fee credit transaction")
	tx, err := txbuilder.NewCloseFCTx(w.userPartitionSystemID, fcb.GetID(), userPartitionTimeout, fcb.Value, targetBill.GetID(), targetBill.TxHash, k)
	if err != nil {
		return nil, nil, err
	}
	closeFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, tx, k.PubKey)
	if err != nil {
		return nil, nil, err
	}
	return closeFCProof, targetBill, nil
}

func (w *FeeManager) sendReclaimFCTx(ctx context.Context, closeFCProof *wallet.Proof, targetBill *wallet.Bill, accountKey *account.AccountKey) (*wallet.Proof, error) {
	moneyTimeout, err := w.getMoneyTimeout(ctx)
	if err != nil {
		return nil, err
	}
	log.Info("sending reclaim fee credit transaction")
	reclaimFCTx, err := txbuilder.NewReclaimFCTx(w.moneySystemID, targetBill.GetID(), moneyTimeout, closeFCProof, targetBill.TxHash, accountKey)
	if err != nil {
		return nil, err
	}
	return w.moneyTxPublisher.SendTx(ctx, reclaimFCTx, accountKey.PubKey)
}

func (w *FeeManager) getSortedBills(k *account.AccountKey) ([]*wallet.Bill, error) {
	bills, err := w.moneyBackendClient.GetBills(k.PubKey)
	if err != nil {
		return nil, err
	}
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value > bills[j].Value
	})
	return bills, nil
}

func (w *FeeManager) getMoneyTimeout(ctx context.Context) (uint64, error) {
	moneyRoundNumber, err := w.moneyBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return 0, err
	}
	return moneyRoundNumber + txTimeoutBlockCount, nil
}

func (w *FeeManager) getUnaddedTransferFCProof(ctx context.Context, accountKey *account.AccountKey, fcb *wallet.Bill, userPartitionRoundNumber uint64, amount uint64) (*wallet.Proof, error) {
	// fetch last transferFC
	txr, err := w.moneyBackendClient.GetLockedFeeCredit(ctx, w.userPartitionSystemID, accountKey.PrivKeyHash)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch locked fee credit: %w", err)
	}
	if txr == nil {
		return nil, nil
	}
	txo := txr.TransactionOrder
	attr := &transactions.TransferFeeCreditAttributes{}
	if err = txo.UnmarshalAttributes(attr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tx attributes: %w", err)
	}

	// the transferFC.nonce must match the FeeCreditRecord.Hash i.e. no new AddFC has been confirmed in the meanwhile
	if !bytes.Equal(attr.Nonce, fcb.GetLastAddFCTxHash()) {
		return nil, nil
	}
	// and the timeout cannot be exceeded
	if userPartitionRoundNumber < attr.EarliestAdditionTime || userPartitionRoundNumber >= attr.LatestAdditionTime {
		return nil, nil
	}
	// and the amount must match
	if attr.Amount != amount {
		return nil, fmt.Errorf("invalid amount: locked fee credit exists for amount %d but user specified %d", attr.Amount, amount)
	}
	log.Info(fmt.Sprintf("found existing transferFC: partition=%X earliest=%d latest=%d current=%d nonce=%X lastAddFCTxHash=%X",
		w.userPartitionSystemID, attr.EarliestAdditionTime, attr.LatestAdditionTime, userPartitionRoundNumber, attr.Nonce, fcb.GetLastAddFCTxHash()))

	transferFCProof, err := w.moneyBackendClient.GetTxProof(ctx, txo.UnitID(), txo.Hash(crypto.SHA256))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch tx proof: %w", err)
	}
	return transferFCProof, nil
}

func (w *FeeManager) getUnreclaimedCloseFCProof(ctx context.Context, accountKey *account.AccountKey, bills []*wallet.Bill) (*wallet.Proof, *wallet.Bill, error) {
	// fetch last closeFC
	txr, err := w.userPartitionBackendClient.GetClosedFeeCredit(ctx, accountKey.PrivKeyHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch locked fee credit: %w", err)
	}
	if txr == nil {
		return nil, nil, nil
	}
	txo := txr.TransactionOrder
	attr := &transactions.CloseFeeCreditAttributes{}
	if err = txo.UnmarshalAttributes(attr); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal tx attributes: %w", err)
	}

	// check if the closeFC has been reclaimed or the target unit has been spent
	var targetUnit *wallet.Bill
	for _, b := range bills {
		if bytes.Equal(b.Id, attr.TargetUnitID) && bytes.Equal(b.TxHash, attr.Nonce) {
			targetUnit = b
			break
		}
	}
	if targetUnit == nil {
		return nil, nil, nil
	}

	// fetch proof for closeFC
	log.Info(fmt.Sprintf("found unreclaimed closeFC: partition=%X nonce=%X targetUnitID=%X",
		w.userPartitionSystemID, attr.Nonce, attr.TargetUnitID))

	closeFCProof, err := w.userPartitionBackendClient.GetTxProof(ctx, txo.UnitID(), txo.Hash(crypto.SHA256))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch tx proof: %w", err)
	}
	return closeFCProof, targetUnit, nil
}

func (c *AddFeeCmd) isValid() error {
	if c.Amount == 0 {
		return errors.New("fee credit amount must be positive")
	}
	return nil
}
