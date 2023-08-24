package fees

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

const (
	maxFee                       = uint64(1)
	txTimeoutBlockCount          = 10
	transferFCLatestAdditionTime = 65536 // relative timeout after which transferFC unit becomes unusable
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
		GetClosedFeeCredit(ctx context.Context, fcbID []byte) (*types.TransactionRecord, error)
	}

	MoneyClient interface {
		GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error)
		GetLockedFeeCredit(ctx context.Context, systemID []byte, fcbID []byte) (*types.TransactionRecord, error)
		PartitionDataProvider
	}

	PartitionNewFeeCreditRecordID func(shardPart, unitPart []byte) types.UnitID

	FeeManager struct {
		am         account.Manager
		unitLocker UnitLocker

		// money partition fields
		moneySystemID      []byte
		moneyTxPublisher   TxPublisher
		moneyBackendClient MoneyClient

		// user partition fields
		userPartitionSystemID             []byte
		userPartitionTxPublisher          TxPublisher
		userPartitionBackendClient        PartitionDataProvider
		userPartitionNewFeeCreditRecordID PartitionNewFeeCreditRecordID
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
	partitionNewFeeCreditRecordID PartitionNewFeeCreditRecordID,
) *FeeManager {
	return &FeeManager{
		am:                                am,
		unitLocker:                        unitLocker,
		moneySystemID:                     moneySystemID,
		moneyTxPublisher:                  moneyTxPublisher,
		moneyBackendClient:                moneyBackendClient,
		userPartitionSystemID:             partitionSystemID,
		userPartitionTxPublisher:          partitionTxPublisher,
		userPartitionBackendClient:        partitionBackendClient,
		userPartitionNewFeeCreditRecordID: partitionNewFeeCreditRecordID,
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
		return nil, fmt.Errorf("failed to load account key: %w", err)
	}

	// if partial reclaim exists, ask user to finish the reclaim process first
	lockedBills, err := w.unitLocker.GetUnits(accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch locked units: %w", err)
	}
	lockedReclaimUnit := w.getLockedBillByReason(lockedBills, unitlock.LockReasonReclaimFees)
	if lockedReclaimUnit != nil {
		return nil, errors.New("wallet contains unreclaimed fee credit, run the reclaim command before adding fee credit")
	}

	// fetch fee credit bill
	fcb, err := w.GetFeeCredit(ctx, GetFeeCreditCmd{AccountIndex: cmd.AccountIndex})
	if err != nil {
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
	moneyTimeout := moneyRoundNumber + txTimeoutBlockCount
	userPartitionTimeout := userPartitionRoundNumber + txTimeoutBlockCount
	latestAdditionTime := userPartitionRoundNumber + transferFCLatestAdditionTime

	// check for any pending add fee credit transactions
	transferFCProof, addFCProof, err := w.getAddFeeCreditState(ctx, lockedBills, moneyRoundNumber, userPartitionRoundNumber, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to find existing add fee credit state: %w", err)
	}

	// addFC was successfully confirmed => everything ok
	if addFCProof != nil {
		return &AddFeeCmdResponse{TransferFC: transferFCProof, AddFC: addFCProof}, nil
	}

	// if no existing transferFC found
	if transferFCProof == nil {
		// find any unlocked bill and send transferFC
		transferFCProof, err = w.sendTransferFC(ctx, cmd, accountKey, fcb, moneyTimeout, userPartitionRoundNumber, latestAdditionTime)
		if err != nil {
			return nil, fmt.Errorf("failed to send transferFC: %w", err)
		}

		// update userPartitionTimeout as it may have taken a while to confirm transferFC
		userPartitionRoundNumber, err = w.userPartitionBackendClient.GetRoundNumber(ctx)
		if err != nil {
			return nil, err
		}
		userPartitionTimeout = userPartitionTimeout + txTimeoutBlockCount
	}

	// create addFC transaction
	fcrID := w.userPartitionNewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256)
	addFCTx, err := txbuilder.NewAddFCTx(fcrID, transferFCProof, accountKey, w.userPartitionSystemID, userPartitionTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create addFC transaction: %w", err)
	}
	lockedUnit, err := w.updateLockedUnitTx(
		accountKey.PubKey,
		transferFCProof.TxRecord.TransactionOrder.UnitID(),
		unitlock.NewTransaction(addFCTx),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update locked unit for addFC tx: %w", err)
	}

	log.Info("sending add fee credit transaction")
	addFCProof, err = w.userPartitionTxPublisher.SendTx(ctx, addFCTx, accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to send addFC transaction: %w", err)
	}
	if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedUnit.UnitID); err != nil {
		return nil, fmt.Errorf("failed to unlock target fee credit bill: %w", err)
	}
	return &AddFeeCmdResponse{TransferFC: transferFCProof, AddFC: addFCProof}, nil
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
	lockedAddBill := w.getLockedBillByReason(lockedBills, unitlock.LockReasonAddFees)
	if lockedAddBill != nil {
		return nil, errors.New("wallet contains unadded fee credit, run the add command before reclaiming fee credit")
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

	log.Info("sending add fee credit transaction")
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
	return w.userPartitionBackendClient.GetFeeCreditBill(ctx, w.userPartitionNewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256))
}

func (w *FeeManager) Close() {
	w.moneyTxPublisher.Close()
	w.userPartitionTxPublisher.Close()
	_ = w.unitLocker.Close()
}

func (w *FeeManager) sendTransferFC(ctx context.Context, cmd AddFeeCmd, accountKey *account.AccountKey, fcb *wallet.Bill, timeout, earliestAdditionTime, latestAdditionTime uint64) (proof *wallet.Proof, err error) {
	bills, err := w.getSortedBills(ctx, accountKey)
	if err != nil {
		return nil, err
	}
	// verify at least one bill in wallet
	if len(bills) == 0 {
		return nil, errors.New("wallet does not contain any bills")
	}

	// find unlocked bill
	targetBill, err := w.getFirstUnlockedBill(accountKey.PubKey, bills)
	if err != nil {
		return nil, err
	}
	// verify bill is large enough for required amount
	if targetBill.Value < cmd.Amount {
		return nil, errors.New("wallet does not have a bill large enough for fee transfer")
	}

	// create transferFC
	log.Info("sending transfer fee credit transaction")
	targetRecordID := w.userPartitionNewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256)
	tx, err := txbuilder.NewTransferFCTx(cmd.Amount, targetRecordID, fcb.GetLastAddFCTxHash(), accountKey, w.moneySystemID, w.userPartitionSystemID, targetBill, timeout, earliestAdditionTime, latestAdditionTime)
	if err != nil {
		return nil, err
	}

	// lock target bill before sending transferFC
	err = w.unitLocker.LockUnit(unitlock.NewLockedUnit(
		accountKey.PubKey,
		targetBill.Id,
		targetBill.TxHash,
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
	fcb, err := w.userPartitionBackendClient.GetFeeCreditBill(ctx, w.userPartitionNewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256))

	if err != nil {
		return nil, err
	}
	if fcb.GetValue() < maxFee {
		return nil, errors.New("insufficient fee credit balance")
	}
	// send closeFC tx to user partition
	log.Info("sending close fee credit transaction")
	tx, err := txbuilder.NewCloseFCTx(w.userPartitionSystemID, fcb.GetID(), userPartitionTimeout, fcb.Value, targetBill.Id, targetBill.TxHash, accountKey)
	if err != nil {
		return nil, err
	}
	// lock target bill before sending closeFC
	err = w.unitLocker.LockUnit(unitlock.NewLockedUnit(
		accountKey.PubKey,
		targetBill.Id,
		targetBill.TxHash,
		unitlock.LockReasonReclaimFees,
		unitlock.NewTransaction(tx),
	))
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

func (w *FeeManager) getAddFeeCreditState(ctx context.Context, lockedBills []*unitlock.LockedUnit, moneyRoundNumber uint64, userPartitionRoundNumber uint64, accountKey *account.AccountKey) (*wallet.Proof, *wallet.Proof, error) {
	var transferFCProof *wallet.Proof
	var addFCProof *wallet.Proof
	var err error
	lockedFeeBill := w.getLockedBillByReason(lockedBills, unitlock.LockReasonAddFees)
	if lockedFeeBill != nil {
		if lockedFeeBill.Transactions[0].TxOrder.PayloadType() == transactions.PayloadTypeTransferFeeCredit {
			transferFCProof, err = w.handleLockedTransferFC(ctx, lockedFeeBill, moneyRoundNumber, accountKey)
			if err != nil {
				return nil, nil, err
			}
		} else if lockedFeeBill.Transactions[0].TxOrder.PayloadType() == transactions.PayloadTypeAddFeeCredit {
			transferFCProof, addFCProof, err = w.handleLockedAddFC(ctx, lockedFeeBill, userPartitionRoundNumber, accountKey)
			if err != nil {
				return nil, nil, err
			}
		} else {
			return nil, nil, fmt.Errorf("found locked bill for creating fee credit but with invalid tx payload type '%s'", lockedFeeBill.Transactions[0].TxOrder.PayloadType())
		}
	}
	return transferFCProof, addFCProof, nil
}

func (w *FeeManager) getReclaimFeeCreditState(ctx context.Context, bills []*wallet.Bill, lockedBills []*unitlock.LockedUnit, userPartitionRoundNumber uint64, accountKey *account.AccountKey) (*wallet.Proof, *wallet.Proof, error) {
	var closeFCProof *wallet.Proof
	var reclaimFCProof *wallet.Proof
	var err error
	lockedFeeBill := w.getLockedBillByReason(lockedBills, unitlock.LockReasonReclaimFees)
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
		log.Info("found proof for pending transferFC")
		return proof, nil
	} else {
		// if no proof => tx either failed or still pending
		if moneyRoundNumber < lockedFeeBill.Transactions[0].TxOrder.Timeout() {
			log.Info("re-broadcasting transferFC")
			tx, err := w.moneyTxPublisher.SendTx(ctx, lockedFeeBill.Transactions[0].TxOrder, accountKey.PubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to re-broadcast transferFC: %w", err)
			}
			return tx, err
		} else {
			log.Info("transferFC not confirmed in time, unlocking transferFC unit")
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
			log.Info("re-broadcasting addFC")
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
				log.Info("addFC timed out, but transferFC still usable, sending new addFC transaction")
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
		log.Info("found proof for pending closeFC")
		return proof, nil
	} else {
		// if no proof => tx either failed or still pending
		if partitionRoundNumber < lockedFeeBill.Transactions[0].TxOrder.Timeout() {
			log.Info("re-broadcasting closeFC")
			tx, err := w.userPartitionTxPublisher.SendTx(ctx, lockedFeeBill.Transactions[0].TxOrder, accountKey.PubKey)
			if err != nil {
				return nil, fmt.Errorf("failed to re-broadcast closeFC: %w", err)
			}
			return tx, err
		} else {
			log.Info("closeFC not confirmed in time, unlocking closeFC unit")
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
			log.Info("re-sending reclaimFC")
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
				log.Info("reclaimFC timed out, but locked unit still usable, sending new reclaimFC transaction")
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

func (w *FeeManager) getLockedBillByReason(lockedBills []*unitlock.LockedUnit, reason unitlock.LockReason) *unitlock.LockedUnit {
	for _, lockedBill := range lockedBills {
		if lockedBill.LockReason == reason {
			return lockedBill
		}
	}
	return nil
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
	if c.Amount == 0 {
		return errors.New("fee credit amount must be positive")
	}
	return nil
}
