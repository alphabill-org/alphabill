package fees

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/alphabill-org/alphabill/validator/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/account"
	txbuilder "github.com/alphabill-org/alphabill/validator/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/unitlock"
)

const (
	MinimumFeeAmount             = 3 * txbuilder.MaxFee
	txTimeoutBlockCount          = 10
	transferFCLatestAdditionTime = 65536 // relative timeout after which transferFC unit becomes unusable
)

var (
	ErrMinimumFeeAmount         = errors.New("insufficient fee amount")
	ErrInsufficientBalance      = errors.New("insufficient balance for transaction")
	ErrLockedBillWrongPartition = errors.New("locked bill for wrong partition")
)

type (
	TxPublisher interface {
		SendTx(ctx context.Context, tx *types.TransactionOrder, senderPubKey []byte) (*wallet.Proof, error)
		Close()
	}

	PartitionDataProvider interface {
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error)
		GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	}

	MoneyClient interface {
		GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error)
		GetLockedFeeCredit(ctx context.Context, systemID []byte, fcbID []byte) (*types.TransactionRecord, error)
		PartitionDataProvider
	}
	// GenerateFcrIDFromPublicKey function to generate fee credit UnitID from shard number nad public key
	GenerateFcrIDFromPublicKey func(shardPart, pubKey []byte) types.UnitID

	FeeManager struct {
		am         account.Manager
		unitLocker UnitLocker
		log        *slog.Logger

		// money partition fields
		moneySystemID      []byte
		moneyTxPublisher   TxPublisher
		moneyBackendClient MoneyClient

		// user partition fields
		userPartitionSystemID      []byte
		userPartitionTxPublisher   TxPublisher
		userPartitionBackendClient PartitionDataProvider
		userPartFcrIDFn            GenerateFcrIDFromPublicKey
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
		TransferFC []*wallet.Proof
		AddFC      []*wallet.Proof
	}

	ReclaimFeeCmdResponse struct {
		CloseFC   *wallet.Proof
		ReclaimFC *wallet.Proof
	}

	addFCStateResponse struct {
		lockedUnit *unitlock.LockedUnit
		transferFC *wallet.Proof
		addFC      *wallet.Proof
	}

	UnitLocker interface {
		GetUnits(accountID []byte) ([]*unitlock.LockedUnit, error)
		GetUnit(accountID, unitID []byte) (*unitlock.LockedUnit, error)
		LockUnit(lockedBill *unitlock.LockedUnit) error
		UnlockUnit(accountID, unitID []byte) error
		Close() error
	}
)

// NewFeeManager creates new fee credit manager.
// Parameters:
// - account manager
// - unit locker
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
	unitLocker UnitLocker,
	moneySystemID []byte,
	moneyTxPublisher TxPublisher,
	moneyBackendClient MoneyClient,
	partitionSystemID []byte,
	partitionTxPublisher TxPublisher,
	partitionBackendClient PartitionDataProvider,
	fcrIDFn GenerateFcrIDFromPublicKey,
	log *slog.Logger,
) *FeeManager {
	return &FeeManager{
		am:                         am,
		unitLocker:                 unitLocker,
		moneySystemID:              moneySystemID,
		moneyTxPublisher:           moneyTxPublisher,
		moneyBackendClient:         moneyBackendClient,
		userPartitionSystemID:      partitionSystemID,
		userPartitionTxPublisher:   partitionTxPublisher,
		userPartitionBackendClient: partitionBackendClient,
		userPartFcrIDFn:            fcrIDFn,
		log:                        log,
	}
}

// AddFeeCredit creates fee credit for the given amount.
// If the wallet does not have a bill large enough for the required amount, multiple transferFC+addFC txs are sent until the amount is reached.
// Returns transferFC and/or addFC transaction proofs.
func (w *FeeManager) AddFeeCredit(ctx context.Context, cmd AddFeeCmd) (*AddFeeCmdResponse, error) {
	if err := cmd.isValid(); err != nil {
		return nil, err
	}

	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}

	// if partial reclaim exists, ask user to finish the reclaim process first
	lockedBills, err := w.unitLocker.GetUnits(accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch locked units: %w", err)
	}
	lockedReclaimUnits := w.getLockedBillsByReason(lockedBills, unitlock.LockReasonReclaimFees)
	if len(lockedReclaimUnits) > 0 {
		return nil, errors.New("wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
	}
	if err := w.verifyLockedBillsTargetPartition(lockedBills, unitlock.LockReasonAddFees); err != nil {
		return nil, err
	}

	// fetch round numbers for timeouts
	moneyRoundNumber, err := w.moneyBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}

	// check for any pending add fee credit transactions
	addFCStateResponse, err := w.getAddFeeCreditState(ctx, lockedBills, moneyRoundNumber, userPartitionRoundNumber, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to find existing add fee credit state: %w", err)
	}
	transferFCs := make([]*wallet.Proof, 0, len(addFCStateResponse))
	addFCs := make([]*wallet.Proof, 0, len(addFCStateResponse))
	for _, fcStateResponse := range addFCStateResponse {
		addFCProof, transferFCProof, err := w.handlePendingAddFeeCredit(ctx, accountKey, fcStateResponse.addFC, fcStateResponse.transferFC)
		if err != nil {
			return nil, err
		}
		if transferFCProof != nil {
			transferFCs = append(transferFCs, transferFCProof)
		}
		if addFCProof != nil {
			addFCs = append(addFCs, addFCProof)
		}
	}

	if len(addFCs) == 0 {
		bills, err := w.getSortedBills(ctx, accountKey)
		if err != nil {
			return nil, err
		}
		// verify at least one bill in wallet
		if len(bills) == 0 {
			return nil, errors.New("wallet does not contain any bills")
		}

		balance, err := w.getBalanceOfUnlockedBillsForFeeCredit(accountKey.PubKey, bills)
		if err != nil {
			return nil, err
		}
		if balance < cmd.Amount {
			return nil, ErrInsufficientBalance
		}

		totalTransferredAmount := uint64(0)
		for totalTransferredAmount < cmd.Amount {
			addFCProof, transferFCProof, transferredAmount, err := w.execAddFeeCredit(ctx, cmd, cmd.Amount-totalTransferredAmount, accountKey, bills)
			if err != nil {
				return nil, err
			}
			totalTransferredAmount += transferredAmount
			transferFCs = append(transferFCs, transferFCProof)
			addFCs = append(addFCs, addFCProof)
		}
		err = w.unlockFCUnits(transferFCs, accountKey)
		if err != nil {
			return nil, err
		}
	}

	return &AddFeeCmdResponse{TransferFC: transferFCs, AddFC: addFCs}, nil
}

// ReclaimFeeCredit reclaims fee credit.
// Reclaimed fee credit is added to the largest bill in wallet.
// Returns closeFC and/or reclaimFC transaction proofs.
func (w *FeeManager) ReclaimFeeCredit(ctx context.Context, cmd ReclaimFeeCmd) (*ReclaimFeeCmdResponse, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}
	// if locked bill for add fee credit exists, ask user to finish the add process first
	lockedBills, err := w.unitLocker.GetUnits(accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch locked units: %w", err)
	}
	lockedAddBills := w.getLockedBillsByReason(lockedBills, unitlock.LockReasonAddFees)
	if len(lockedAddBills) > 0 {
		return nil, errors.New("wallet contains unadded fee credit, run the add command before reclaiming fee credit")
	}
	if err := w.verifyLockedBillsTargetPartition(lockedBills, unitlock.LockReasonReclaimFees); err != nil {
		return nil, err
	}

	bills, err := w.getSortedBills(ctx, accountKey)
	if err != nil {
		return nil, err
	}

	// fetch user partition round number for timeouts
	userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, err
	}
	userPartitionTimeout := userPartitionRoundNumber + txTimeoutBlockCount

	closeFCProof, reclaimFCProof, err := w.getReclaimFeeCreditState(ctx, bills, lockedBills, userPartitionRoundNumber, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to find existing add fee credit state: %w", err)
	}

	// reclaimFC was successfully confirmed => everything ok
	if reclaimFCProof != nil {
		return &ReclaimFeeCmdResponse{CloseFC: closeFCProof, ReclaimFC: reclaimFCProof}, nil
	}

	// if no existing closeFC found
	if closeFCProof == nil {
		// find any unlocked bill and send closeFC
		closeFCProof, err = w.sendCloseFC(ctx, bills, accountKey, userPartitionTimeout)
		if err != nil {
			return nil, fmt.Errorf("failed to send closeFC: %w", err)
		}
	}

	moneyTimeout, err := w.getMoneyTimeout(ctx)
	if err != nil {
		return nil, err
	}

	var closeFCAttr *transactions.CloseFeeCreditAttributes
	if err := closeFCProof.TxRecord.TransactionOrder.UnmarshalAttributes(&closeFCAttr); err != nil {
		return nil, fmt.Errorf("failed to unmarshal closeFC attributes: %w", err)
	}

	reclaimFC, err := txbuilder.NewReclaimFCTx(w.moneySystemID, closeFCAttr.TargetUnitID, moneyTimeout, closeFCProof, closeFCAttr.TargetUnitBacklink, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create reclaimFC transaction: %w", err)
	}
	lockedUnit, err := w.updateLockedUnitTx(
		accountKey.PubKey,
		closeFCAttr.TargetUnitID,
		unitlock.NewTransaction(reclaimFC),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update locked unit for reclaim tx: %w", err)
	}

	w.log.InfoContext(ctx, "sending add fee credit transaction")
	reclaimFCProof, err = w.moneyTxPublisher.SendTx(ctx, reclaimFC, accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to send reclaimFC transaction: %w", err)
	}
	if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedUnit.UnitID); err != nil {
		return nil, fmt.Errorf("failed to unlock target fee credit bill: %w", err)
	}
	return &ReclaimFeeCmdResponse{CloseFC: closeFCProof, ReclaimFC: reclaimFCProof}, nil
}

// GetFeeCredit returns fee credit bill for given account,
// can return nil if fee credit bill has not been created yet.
func (w *FeeManager) GetFeeCredit(ctx context.Context, cmd GetFeeCreditCmd) (*wallet.Bill, error) {
	accountKey, err := w.am.GetAccountKey(cmd.AccountIndex)
	if err != nil {
		return nil, err
	}
	fcrID := w.userPartFcrIDFn(nil, accountKey.PubKey)
	return w.userPartitionBackendClient.GetFeeCreditBill(ctx, fcrID)
}

func (w *FeeManager) Close() {
	w.moneyTxPublisher.Close()
	w.userPartitionTxPublisher.Close()
	_ = w.unitLocker.Close()
}

func (w *FeeManager) sendTransferFC(ctx context.Context, amount uint64, accountKey *account.AccountKey, fcb *wallet.Bill, bills []*wallet.Bill, timeout, earliestAdditionTime, latestAdditionTime uint64) (proof *wallet.Proof, err error) {
	// find unlocked bill
	targetBill, err := w.getFirstUnlockedBill(accountKey.PubKey, bills)
	if err != nil {
		return nil, err
	}

	// create transferFC
	w.log.InfoContext(ctx, "sending transfer fee credit transaction")
	targetRecordID := w.userPartFcrIDFn(nil, accountKey.PubKey)
	tx, err := txbuilder.NewTransferFCTx(
		min(amount, targetBill.Value),
		targetRecordID,
		fcb.GetLastAddFCTxHash(),
		accountKey,
		w.moneySystemID,
		w.userPartitionSystemID,
		targetBill,
		timeout,
		earliestAdditionTime,
		latestAdditionTime,
	)
	if err != nil {
		return nil, err
	}

	// lock target bill before sending transferFC
	err = w.unitLocker.LockUnit(unitlock.NewLockedUnit(
		accountKey.PubKey,
		targetBill.Id,
		targetBill.TxHash,
		w.userPartitionSystemID,
		unitlock.LockReasonAddFees,
		unitlock.NewTransaction(tx)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to lock bill: %w", err)
	}
	// send transferFC to money partition
	return w.moneyTxPublisher.SendTx(ctx, tx, accountKey.PubKey)
}

func (w *FeeManager) sendCloseFC(ctx context.Context, bills []*wallet.Bill, accountKey *account.AccountKey, userPartitionTimeout uint64) (*wallet.Proof, error) {
	if len(bills) == 0 {
		return nil, errors.New("wallet must have a source bill to which to add reclaimed fee credits")
	}
	// find unlocked bill
	targetBill, err := w.getFirstUnlockedBill(accountKey.PubKey, bills)
	if err != nil {
		return nil, err
	}
	fcrID := w.userPartFcrIDFn(nil, accountKey.PubKey)
	fcb, err := w.userPartitionBackendClient.GetFeeCreditBill(ctx, fcrID)
	if err != nil {
		return nil, err
	}
	if fcb.GetValue() < MinimumFeeAmount {
		return nil, ErrMinimumFeeAmount
	}
	// send closeFC tx to user partition
	w.log.InfoContext(ctx, "sending close fee credit transaction")
	tx, err := txbuilder.NewCloseFCTx(w.userPartitionSystemID, fcb.GetID(), userPartitionTimeout, fcb.Value, targetBill.Id, targetBill.TxHash, accountKey)
	if err != nil {
		return nil, err
	}
	// lock target bill before sending closeFC
	err = w.unitLocker.LockUnit(unitlock.NewLockedUnit(
		accountKey.PubKey,
		targetBill.Id,
		targetBill.TxHash,
		w.userPartitionSystemID,
		unitlock.LockReasonReclaimFees,
		unitlock.NewTransaction(tx),
	))
	if err != nil {
		return nil, fmt.Errorf("locking target bill: %w", err)
	}
	closeFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, tx, accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	return closeFCProof, nil
}

func (w *FeeManager) getSortedBills(ctx context.Context, k *account.AccountKey) ([]*wallet.Bill, error) {
	bills, err := w.moneyBackendClient.GetBills(ctx, k.PubKey)
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

func (w *FeeManager) handlePendingAddFeeCredit(ctx context.Context, accountKey *account.AccountKey, addFC *wallet.Proof, transferFC *wallet.Proof) (*wallet.Proof, *wallet.Proof, error) {
	// addFC was successfully confirmed => everything ok
	if addFC != nil {
		return addFC, transferFC, nil
	}
	if transferFC != nil {
		// update userPartitionTimeout as it may have taken a while to confirm transferFC
		userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
		if err != nil {
			return nil, nil, err
		}
		userPartitionTimeout := userPartitionRoundNumber + txTimeoutBlockCount
		// create addFC transaction
		fcrID := w.userPartFcrIDFn(nil, accountKey.PubKey)
		addFCTx, err := txbuilder.NewAddFCTx(fcrID, transferFC, accountKey, w.userPartitionSystemID, userPartitionTimeout)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create addFC transaction: %w", err)
		}
		lockedUnit, err := w.updateLockedUnitTx(
			accountKey.PubKey,
			transferFC.TxRecord.TransactionOrder.UnitID(),
			unitlock.NewTransaction(addFCTx),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to update locked unit for addFC tx: %w", err)
		}

		w.log.InfoContext(ctx, "sending add fee credit transaction")
		addFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, addFCTx, accountKey.PubKey)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to send addFC transaction: %w", err)
		}
		if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedUnit.UnitID); err != nil {
			return nil, nil, fmt.Errorf("failed to unlock target fee credit bill: %w", err)
		}
		return addFCProof, transferFC, nil
	}
	return nil, nil, nil
}

func (w *FeeManager) execAddFeeCredit(ctx context.Context, cmd AddFeeCmd, amount uint64, accountKey *account.AccountKey, bills []*wallet.Bill) (*wallet.Proof, *wallet.Proof, uint64, error) {
	// fetch fee credit bill
	fcb, err := w.GetFeeCredit(ctx, GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
		return nil, nil, 0, err
	}

	// fetch round numbers for timeouts
	moneyRoundNumber, err := w.moneyBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	userPartitionRoundNumber, err := w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	moneyTimeout := moneyRoundNumber + txTimeoutBlockCount
	latestAdditionTime := userPartitionRoundNumber + transferFCLatestAdditionTime

	// find any unlocked bill and send transferFC
	transferFCProof, err := w.sendTransferFC(ctx, amount, accountKey, fcb, bills, moneyTimeout, userPartitionRoundNumber, latestAdditionTime)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to send transferFC: %w", err)
	}
	transferFCAttr := &transactions.TransferFeeCreditAttributes{}
	if err := transferFCProof.TxRecord.TransactionOrder.UnmarshalAttributes(transferFCAttr); err != nil {
		return nil, nil, 0, fmt.Errorf("failed to unmarshal transferFC attributes: %w", err)
	}

	// update userPartitionTimeout as it may have taken a while to confirm transferFC
	userPartitionRoundNumber, err = w.userPartitionBackendClient.GetRoundNumber(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	userPartitionTimeout := userPartitionRoundNumber + txTimeoutBlockCount
	// create addFC transaction
	fcrID := w.userPartFcrIDFn(nil, accountKey.PubKey)
	addFCTx, err := txbuilder.NewAddFCTx(fcrID, transferFCProof, accountKey, w.userPartitionSystemID, userPartitionTimeout)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to create addFC transaction: %w", err)
	}
	_, err = w.updateLockedUnitTx(
		accountKey.PubKey,
		transferFCProof.TxRecord.TransactionOrder.UnitID(),
		unitlock.NewTransaction(addFCTx),
	)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to update locked unit for addFC tx: %w", err)
	}

	w.log.InfoContext(ctx, "sending add fee credit transaction")
	addFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, addFCTx, accountKey.PubKey)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("failed to send addFC transaction: %w", err)
	}
	return addFCProof, transferFCProof, transferFCAttr.Amount, nil
}

func (w *FeeManager) getAddFeeCreditState(ctx context.Context, lockedBills []*unitlock.LockedUnit, moneyRoundNumber uint64, userPartitionRoundNumber uint64, accountKey *account.AccountKey) ([]*addFCStateResponse, error) {
	response := make([]*addFCStateResponse, 0, len(lockedBills))
	lockedFeeBills := w.getLockedBillsByReason(lockedBills, unitlock.LockReasonAddFees)
	for _, lockedFeeBill := range lockedFeeBills {
		stateResponse := &addFCStateResponse{lockedUnit: lockedFeeBill}
		if lockedFeeBill.Transactions[0].TxOrder.PayloadType() == transactions.PayloadTypeTransferFeeCredit {
			transferFCProof, err := w.handleLockedTransferFC(ctx, lockedFeeBill, moneyRoundNumber, accountKey)
			if err != nil {
				return nil, err
			}
			stateResponse.transferFC = transferFCProof
		} else if lockedFeeBill.Transactions[0].TxOrder.PayloadType() == transactions.PayloadTypeAddFeeCredit {
			transferFCProof, addFCProof, err := w.handleLockedAddFC(ctx, lockedFeeBill, userPartitionRoundNumber, accountKey)
			if err != nil {
				return nil, err
			}
			stateResponse.addFC = addFCProof
			stateResponse.transferFC = transferFCProof
		} else {
			return nil, fmt.Errorf("found locked bill for creating fee credit but with invalid tx payload type '%s'", lockedFeeBill.Transactions[0].TxOrder.PayloadType())
		}
		response = append(response, stateResponse)
	}
	return response, nil
}

func (w *FeeManager) getReclaimFeeCreditState(ctx context.Context, bills []*wallet.Bill, lockedBills []*unitlock.LockedUnit, userPartitionRoundNumber uint64, accountKey *account.AccountKey) (*wallet.Proof, *wallet.Proof, error) {
	var closeFCProof *wallet.Proof
	var reclaimFCProof *wallet.Proof
	var err error
	lockedFeeBill := w.getFirstLockedBillByReason(lockedBills, unitlock.LockReasonReclaimFees)
	if lockedFeeBill != nil {
		if lockedFeeBill.Transactions[0].TxOrder.PayloadType() == transactions.PayloadTypeCloseFeeCredit {
			closeFCProof, err = w.handleLockedCloseFC(ctx, lockedFeeBill, userPartitionRoundNumber, accountKey)
			if err != nil {
				return nil, nil, err
			}
		} else if lockedFeeBill.Transactions[0].TxOrder.PayloadType() == transactions.PayloadTypeReclaimFeeCredit {
			closeFCProof, reclaimFCProof, err = w.handleLockedReclaimFC(ctx, bills, lockedFeeBill, userPartitionRoundNumber, accountKey)
			if err != nil {
				return nil, nil, err
			}
		} else {
			return nil, nil, fmt.Errorf("found locked bill for reclaiming fee credit but with invalid tx payload type '%s'", lockedFeeBill.Transactions[0].TxOrder.PayloadType())
		}
	}
	return closeFCProof, reclaimFCProof, nil
}

func (w *FeeManager) updateLockedUnitTx(accountID, unitID []byte, unit *unitlock.Transaction) (*unitlock.LockedUnit, error) {
	lockedBill, err := w.unitLocker.GetUnit(accountID, unitID)
	if err != nil {
		return nil, fmt.Errorf("failed to load locked bill: %w", err)
	}
	if lockedBill == nil {
		return nil, fmt.Errorf("locked bill cannot be nil: %w", err)
	}
	lockedBill.Transactions = []*unitlock.Transaction{unit}
	err = w.unitLocker.LockUnit(lockedBill)
	if err != nil {
		return nil, fmt.Errorf("failed to update locked bill: %w", err)
	}
	return lockedBill, nil
}

// handleLockedTransferFC handles the locked transferFC unit, it either
// 1. returns proof for currently pending transferFC if confirmed
// 2. re-sends transferFC if not yet timed out (alternatively could return error, ask user to wait)
// 3. returns error if transferFC timed out
func (w *FeeManager) handleLockedTransferFC(ctx context.Context, lockedFeeBill *unitlock.LockedUnit, moneyRoundNumber uint64, accountKey *account.AccountKey) (*wallet.Proof, error) {
	proof, err := w.moneyBackendClient.GetTxProof(ctx, lockedFeeBill.UnitID, lockedFeeBill.Transactions[0].TxHash)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transferFC proof: %w", err)
	}
	// if we have proof => tx was confirmed
	if proof != nil {
		w.log.InfoContext(ctx, "found proof for pending transferFC")
		return proof, nil
	} else {
		// if no proof => tx either failed or still pending
		if moneyRoundNumber < lockedFeeBill.Transactions[0].TxOrder.Timeout() {
			w.log.InfoContext(ctx, "re-broadcasting transferFC")
			tx, err := w.moneyTxPublisher.SendTx(ctx, lockedFeeBill.Transactions[0].TxOrder, accountKey.PubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to re-broadcast transferFC: %w", err)
			}
			return tx, err
		} else {
			w.log.InfoContext(ctx, "transferFC not confirmed in time, unlocking transferFC unit")
			if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
				return nil, fmt.Errorf("failed to unlock transferFC bill: %w", err)
			}
			return nil, nil
		}
	}
}

// handleLockedAddFC handles the locked addFC unit, it either
// 1. returns proof for currently pending addFC if confirmed
// 2. re-sends addFC if addFC is not yet timed out (alternatively could return error, ask user to wait)
// 3. returns transferFC proof if addFC is timed out but target bill still usable
// 4. returns error if addFC timed out and target bill is no longer usable
func (w *FeeManager) handleLockedAddFC(ctx context.Context, lockedFeeBill *unitlock.LockedUnit, userPartitionRoundNumber uint64, accountKey *account.AccountKey) (*wallet.Proof, *wallet.Proof, error) {
	// if we're waiting on addFC check if the tx has already been confirmed or failed
	proof, err := w.userPartitionBackendClient.GetTxProof(ctx, lockedFeeBill.UnitID, lockedFeeBill.Transactions[0].TxHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch addFC proof: %w", err)
	}
	// if we have proof => tx was confirmed
	if proof != nil {
		if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
			return nil, nil, fmt.Errorf("failed to unlock addFC bill after finding confirmed addFC: %w", err)
		}
		return nil, proof, nil
	} else {
		// if no proof => tx either failed or still pending
		if userPartitionRoundNumber < lockedFeeBill.Transactions[0].TxOrder.Timeout() {
			w.log.InfoContext(ctx, "re-broadcasting addFC")
			addFCProof, err := w.userPartitionTxPublisher.SendTx(ctx, lockedFeeBill.Transactions[0].TxOrder, accountKey.PubKey)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to re-broadcast addFC: %w", err)
			}
			if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
				return nil, nil, fmt.Errorf("failed to unlock addFC bill after re-sending and confirming addFC: %w", err)
			}
			return nil, addFCProof, nil
		} else {
			// check if locked bill still usable
			// if yes => create new addFC
			// if not => log money lost error
			transferFCAttr, addFCAttr, err := w.unmarshalTransferFC(lockedFeeBill)
			if err != nil {
				return nil, nil, err
			}
			if userPartitionRoundNumber < transferFCAttr.LatestAdditionTime {
				w.log.InfoContext(ctx, "addFC timed out, but transferFC still usable, sending new addFC transaction")
				return &wallet.Proof{
					TxRecord: addFCAttr.FeeCreditTransfer,
					TxProof:  addFCAttr.FeeCreditTransferProof,
				}, nil, nil
			} else {
				if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
					return nil, nil, fmt.Errorf("failed to unlock target fee credit bill: %w", err)
				}
				return nil, nil, errors.New("transferFC latestAdditionTime exceeded, locked fee credit is no longer usable")
			}
		}
	}
}

// handleLockedCloseFC handles the locked closeFC unit, it either
// 1. returns proof for currently pending closeFC if confirmed
// 2. re-sends closeFC if not yet timed out (alternatively could return error, ask user to wait)
// 3. returns error if closeFC timed out
func (w *FeeManager) handleLockedCloseFC(ctx context.Context, lockedFeeBill *unitlock.LockedUnit, partitionRoundNumber uint64, accountKey *account.AccountKey) (*wallet.Proof, error) {
	proof, err := w.userPartitionBackendClient.GetTxProof(ctx, lockedFeeBill.UnitID, lockedFeeBill.Transactions[0].TxHash)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch closeFC proof: %w", err)
	}
	// if we have proof => tx was confirmed
	if proof != nil {
		w.log.InfoContext(ctx, "found proof for pending closeFC")
		return proof, nil
	} else {
		// if no proof => tx either failed or still pending
		if partitionRoundNumber < lockedFeeBill.Transactions[0].TxOrder.Timeout() {
			w.log.InfoContext(ctx, "re-broadcasting closeFC")
			tx, err := w.userPartitionTxPublisher.SendTx(ctx, lockedFeeBill.Transactions[0].TxOrder, accountKey.PubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to re-broadcast closeFC: %w", err)
			}
			return tx, err
		} else {
			w.log.InfoContext(ctx, "closeFC not confirmed in time, unlocking closeFC unit")
			if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
				return nil, fmt.Errorf("failed to unlock closeFC unit: %w", err)
			}
			return nil, nil
		}
	}
}

// handleLockedReclaimFC handles the locked reclaimFC unit, it either
// 1. returns proof for currently pending reclaimFC if reclaimFC is confirmed
// 2. re-sends reclaimFC if reclaimFC is not yet timed out (alternatively could return error, ask user to wait)
// 3. returns closeFC proof if reclaimFC is timed out but target bill still usable
// 4. returns error if reclaimFC timed out and target bill is no longer usable
func (w *FeeManager) handleLockedReclaimFC(ctx context.Context, bills []*wallet.Bill, lockedFeeBill *unitlock.LockedUnit, moneyRoundNumber uint64, accountKey *account.AccountKey) (*wallet.Proof, *wallet.Proof, error) {
	// if we're waiting on reclaimFC check if the tx has already been confirmed or failed
	proof, err := w.moneyBackendClient.GetTxProof(ctx, lockedFeeBill.UnitID, lockedFeeBill.Transactions[0].TxHash)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch reclaimFC proof: %w", err)
	}
	if proof != nil {
		// if we have proof => tx was confirmed
		if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
			return nil, nil, fmt.Errorf("failed to unlock reclaimFC bill after finding confirmed reclaimFC: %w", err)
		}
		return nil, proof, nil
	} else {
		// if no proof => tx either failed or still pending
		if moneyRoundNumber < lockedFeeBill.Transactions[0].TxOrder.Timeout() {
			w.log.InfoContext(ctx, "re-sending reclaimFC")
			reclaimFCProof, err := w.moneyTxPublisher.SendTx(ctx, lockedFeeBill.Transactions[0].TxOrder, accountKey.PubKey)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to re-broadcast reclaimFC: %w", err)
			}
			if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
				return nil, nil, fmt.Errorf("failed to unlock reclaimFC bill after re-sending and confirming reclaimFC: %w", err)
			}
			return nil, reclaimFCProof, nil
		} else {
			// check if locked bill still usable
			// if yes => create new reclaimFC
			// if not => log money lost error
			if targetUnit := w.getBillByIdAndHash(bills, lockedFeeBill.UnitID, lockedFeeBill.TxHash); targetUnit != nil {
				w.log.InfoContext(ctx, "reclaimFC timed out, but locked unit still usable, sending new reclaimFC transaction")
				reclaimFCAttr := &transactions.ReclaimFeeCreditAttributes{}
				if err := lockedFeeBill.Transactions[0].TxOrder.UnmarshalAttributes(reclaimFCAttr); err != nil {
					return nil, nil, fmt.Errorf("failed to unmarshal reclaimFC attributes: %w", err)
				}
				return &wallet.Proof{
					TxRecord: reclaimFCAttr.CloseFeeCreditTransfer,
					TxProof:  reclaimFCAttr.CloseFeeCreditProof,
				}, nil, nil
			} else {
				if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedFeeBill.UnitID); err != nil {
					return nil, nil, fmt.Errorf("failed to unlock target fee credit bill: %w", err)
				}
				return nil, nil, errors.New("reclaimFC target unit is no longer usable")
			}
		}
	}
}

func (w *FeeManager) getBillByIdAndHash(bills []*wallet.Bill, unitID []byte, txHash []byte) *wallet.Bill {
	for _, b := range bills {
		if bytes.Equal(b.Id, unitID) && bytes.Equal(b.TxHash, txHash) {
			return b
		}
	}
	return nil
}

func (w *FeeManager) getFirstLockedBillByReason(lockedBills []*unitlock.LockedUnit, reason unitlock.LockReason) *unitlock.LockedUnit {
	for _, lockedBill := range lockedBills {
		if lockedBill.LockReason == reason {
			return lockedBill
		}
	}
	return nil
}

func (w *FeeManager) verifyLockedBillsTargetPartition(lockedBills []*unitlock.LockedUnit, reason unitlock.LockReason) error {
	for _, lockedBill := range lockedBills {
		if lockedBill.LockReason == reason && !bytes.Equal(lockedBill.SystemID, w.userPartitionSystemID) {
			return fmt.Errorf("%w: lockedBillSystemID=%X, providedSystemID=%X",
				ErrLockedBillWrongPartition, lockedBill.SystemID, w.userPartitionSystemID)
		}
	}
	return nil
}

func (w *FeeManager) getLockedBillsByReason(lockedBills []*unitlock.LockedUnit, reason unitlock.LockReason) []*unitlock.LockedUnit {
	filteredLockedBills := make([]*unitlock.LockedUnit, 0, len(lockedBills))
	for _, lockedBill := range lockedBills {
		if lockedBill.LockReason == reason {
			filteredLockedBills = append(filteredLockedBills, lockedBill)
		}
	}
	return filteredLockedBills
}

func (w *FeeManager) getBalanceOfUnlockedBillsForFeeCredit(accountID []byte, bills []*wallet.Bill) (uint64, error) {
	balance := uint64(0)
	for _, b := range bills {
		if b.Value >= MinimumFeeAmount {
			unit, err := w.unitLocker.GetUnit(accountID, b.GetID())
			if err != nil {
				return 0, err
			}
			if unit == nil {
				balance += b.Value
			}
		}
	}
	return balance, nil
}

func (w *FeeManager) getFirstUnlockedBill(accountID []byte, bills []*wallet.Bill) (*wallet.Bill, error) {
	for _, b := range bills {
		unit, err := w.unitLocker.GetUnit(accountID, b.GetID())
		if err != nil {
			return nil, err
		}
		if unit == nil {
			return b, nil
		}
	}
	return nil, errors.New("wallet does not contain any unlocked bills")
}

func (w *FeeManager) unlockFCUnits(addFCs []*wallet.Proof, accountKey *account.AccountKey) error {
	for _, addFC := range addFCs {
		if err := w.unitLocker.UnlockUnit(accountKey.PubKey, addFC.TxRecord.TransactionOrder.UnitID()); err != nil {
			return fmt.Errorf("failed to unlock target fee credit bill: %w", err)
		}
	}
	return nil
}

func (w *FeeManager) unmarshalTransferFC(lockedFeeBill *unitlock.LockedUnit) (*transactions.TransferFeeCreditAttributes, *transactions.AddFeeCreditAttributes, error) {
	addFCAttr := &transactions.AddFeeCreditAttributes{}
	if err := lockedFeeBill.Transactions[0].TxOrder.UnmarshalAttributes(addFCAttr); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal addFC attributes: %w", err)
	}
	transferFCAttr := &transactions.TransferFeeCreditAttributes{}
	if err := addFCAttr.FeeCreditTransfer.TransactionOrder.UnmarshalAttributes(transferFCAttr); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal transferFC attributes: %w", err)
	}
	return transferFCAttr, addFCAttr, nil
}

func (c AddFeeCmd) isValid() error {
	if c.Amount < MinimumFeeAmount {
		return ErrMinimumFeeAmount
	}
	return nil
}
