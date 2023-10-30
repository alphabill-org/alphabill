package dc

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

type (
	DustCollector struct {
		systemID      []byte
		maxBillsPerDC int
		txTimeout     uint64
		backend       BackendAPI
		unitLocker    UnitLocker
		log           *slog.Logger
	}

	DustCollectionResult struct {
		AccountIndex uint64
		FeeSum       uint64
		SwapProof    *wallet.Proof
	}

	BackendAPI interface {
		GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error)
		ListBills(ctx context.Context, pubKey []byte, includeDCBills bool, offsetKey string, limit int) (*backend.ListBillsResponse, error)
		GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(ctx context.Context, unitID types.UnitID) (*wallet.Bill, error)
		PostTransactions(ctx context.Context, pubKey wallet.PubKey, txs *wallet.Transactions) error
		GetTxProof(ctx context.Context, unitID types.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
	}

	UnitLocker interface {
		LockUnit(lockedBill *unitlock.LockedUnit) error
		UnlockUnit(accountID, unitID []byte) error
		GetUnit(accountID, unitID []byte) (*unitlock.LockedUnit, error)
		GetUnits(accountID []byte) ([]*unitlock.LockedUnit, error)
		Close() error
	}
)

func NewDustCollector(systemID []byte, maxBillsPerDC int, txTimeout uint64, backend BackendAPI, unitLocker UnitLocker, log *slog.Logger) *DustCollector {
	return &DustCollector{
		systemID:      systemID,
		maxBillsPerDC: maxBillsPerDC,
		txTimeout:     txTimeout,
		backend:       backend,
		unitLocker:    unitLocker,
		log:           log,
	}
}

// CollectDust joins up to N units into existing target unit, prioritizing small units first. The largest unit is
// selected as the target unit. Returns swap transaction proof or error or nil if there's not enough bills to swap.
func (w *DustCollector) CollectDust(ctx context.Context, accountKey *account.AccountKey) (*DustCollectionResult, error) {
	dcResult, err := w.runExistingDustCollection(ctx, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to run existing swap: %w", err)
	}
	if dcResult != nil {
		return dcResult, nil
	}
	return w.runDustCollection(ctx, accountKey)
}

// runExistingDustCollection executes dust collection process using existing locked bill, if one exists
func (w *DustCollector) runExistingDustCollection(ctx context.Context, accountKey *account.AccountKey) (*DustCollectionResult, error) {
	lockedTargetBill, err := w.getLockedTargetBill(accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	if lockedTargetBill == nil {
		return nil, nil
	}
	w.log.InfoContext(ctx, fmt.Sprintf("locked dc unit found, pubkey=%x", accountKey.PubKey), logger.UnitID(lockedTargetBill.UnitID))

	// verify locked unit not confirmed i.e. swap not already completed
	for _, tx := range lockedTargetBill.Transactions {
		if tx.TxOrder.PayloadType() == money.PayloadTypeSwapDC {
			swapProof, err := w.backend.GetTxProof(ctx, tx.TxOrder.UnitID(), tx.TxHash)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch proof: %w", err)
			}
			if swapProof != nil {
				// if it's confirmed unlock the unit and return swap proof
				if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedTargetBill.UnitID); err != nil {
					return nil, fmt.Errorf("failed to unlock unit: %w", err)
				}
				dcProofs, err := w.fetchDCProofsForTargetUnit(ctx, accountKey, lockedTargetBill)
				if err != nil {
					return nil, fmt.Errorf("failed to fetch proofs for target unit: %w", err)
				}
				return &DustCollectionResult{SwapProof: swapProof, FeeSum: swapProof.TxRecord.ServerMetadata.ActualFee + w.sumPaidFees(dcProofs)}, nil
			}
		}
	}

	// verify locked unit still usable
	valid, err := w.verifyLockedUnitValid(ctx, accountKey, lockedTargetBill)
	if err != nil {
		return nil, err
	}
	if !valid {
		w.log.WarnContext(ctx, "locked unit no longer valid, unlocking the unit", logger.UnitID(lockedTargetBill.UnitID))
		if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedTargetBill.UnitID); err != nil {
			return nil, fmt.Errorf("failed to unlock unit: %w", err)
		}
		return nil, nil
	}
	w.log.InfoContext(ctx, "locked unit still valid", logger.UnitID(lockedTargetBill.UnitID))

	// wait for tx timeouts
	for _, tx := range lockedTargetBill.Transactions {
		_, err := w.waitForConf(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to wait for tx confirmation: %w", err)
		}
	}

	// after waiting for confirmations, fetch all dc bills for given target unit
	proofs, err := w.fetchDCProofsForTargetUnit(ctx, accountKey, lockedTargetBill)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch proofs for target unit: %w", err)
	}
	// if any proof found, swap them
	if len(proofs) > 0 {
		swapProof, err := w.swapDCBills(ctx, accountKey, proofs, lockedTargetBill)
		if err != nil {
			return nil, err
		}
		return &DustCollectionResult{SwapProof: swapProof, FeeSum: swapProof.TxRecord.ServerMetadata.ActualFee + w.sumPaidFees(proofs)}, nil
	}
	// if no proofs found, run normal DC
	w.log.InfoContext(ctx, "no dust txs confirmed, unlocking target unit", logger.UnitID(lockedTargetBill.UnitID))
	if err := w.unitLocker.UnlockUnit(accountKey.PubKey, lockedTargetBill.UnitID); err != nil {
		return nil, fmt.Errorf("failed to unlock unit: %w", err)
	}
	return nil, nil
}

func (w *DustCollector) runDustCollection(ctx context.Context, accountKey *account.AccountKey) (*DustCollectionResult, error) {
	// fetch non-dc bills
	bills, err := w.backend.GetBills(ctx, accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	bills, err = w.filterLockedBills(bills, accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	if len(bills) < 2 {
		w.log.InfoContext(ctx, "account has less than two unlocked bills, skipping dust collection")
		return &DustCollectionResult{}, nil
	}
	// sort bills by value smallest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value < bills[j].Value
	})
	// use the largest bill as target
	targetBill := bills[len(bills)-1]
	billCountToSwap := min(w.maxBillsPerDC, len(bills)-1)
	return w.submitDCBatch(ctx, accountKey, targetBill, bills[:billCountToSwap])
}

func (w *DustCollector) filterLockedBills(bills []*wallet.Bill, accountID []byte) ([]*wallet.Bill, error) {
	var nonLockedBills []*wallet.Bill
	for _, b := range bills {
		exists, err := w.unitLocker.GetUnit(accountID, b.GetID())
		if err != nil {
			return nil, fmt.Errorf("failed to load locked unit: %w", err)
		}
		if exists == nil {
			nonLockedBills = append(nonLockedBills, b)
		}
	}
	return nonLockedBills, nil
}

func (w *DustCollector) fetchDCProofsForTargetUnit(ctx context.Context, k *account.AccountKey, targetUnit *unitlock.LockedUnit) ([]*wallet.Proof, error) {
	billsResponse, err := w.backend.ListBills(ctx, k.PubKey, true, "", 100)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	var dcProofs []*wallet.Proof
	for _, b := range billsResponse.Bills {
		if bytes.Equal(b.DCTargetUnitID, targetUnit.UnitID) && bytes.Equal(b.DCTargetUnitBacklink, targetUnit.TxHash) {
			proof, err := w.backend.GetTxProof(ctx, b.Id, b.TxHash)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch dc proof: %w", err)
			}
			dcProofs = append(dcProofs, proof)
		}
	}
	return dcProofs, nil
}

func (w *DustCollector) waitForConf(ctx context.Context, tx *unitlock.Transaction) (*wallet.Proof, error) {
	roundNumber, err := w.backend.GetRoundNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch round number: %w", err)
	}
	for roundNumber < tx.TxOrder.Timeout() {
		proof, err := w.backend.GetTxProof(ctx, tx.TxOrder.UnitID(), tx.TxHash)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tx proof: %w", err)
		}
		if proof != nil {
			return proof, nil
		}
		select {
		case <-time.After(time.Second):
			roundNumber, err = w.backend.GetRoundNumber(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch round number: %w", err)
			}
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		}
	}
	return nil, nil
}

func (w *DustCollector) submitDCBatch(ctx context.Context, k *account.AccountKey, targetBill *wallet.Bill, billsToSwap []*wallet.Bill) (*DustCollectionResult, error) {
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}
	fcb, err := w.backend.GetFeeCreditBill(ctx, money.NewFeeCreditRecordID(nil, k.PubKeyHash.Sha256))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	txsCost := tx_builder.MaxFee * uint64(len(billsToSwap)+1) // +1 for swap
	if fcb.GetValue() < txsCost {
		return nil, fmt.Errorf("insufficient fee credit balance for transactions: need at least %d Tema "+
			"but have %d Tema to send swap and %d dust transfer transactions", txsCost, fcb.GetValue(), len(billsToSwap))
	}
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend, w.log)
	for _, b := range billsToSwap {
		tx, err := tx_builder.NewDustTx(k, w.systemID, &wallet.Bill{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}, targetBill, timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to build dust transfer transaction: %w", err)
		}
		dcBatch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}

	// lock target unit
	var lockedUnitTxs []*unitlock.Transaction
	for _, sub := range dcBatch.Submissions() {
		lockedUnitTxs = append(lockedUnitTxs, unitlock.NewTransaction(sub.Transaction))
	}
	lockedTargetUnit := unitlock.NewLockedUnit(k.PubKey, targetBill.Id, targetBill.TxHash, w.systemID, unitlock.LockReasonCollectDust, lockedUnitTxs...)
	err = w.unitLocker.LockUnit(lockedTargetUnit)
	if err != nil {
		return nil, fmt.Errorf("failed to lock unit for dc batch: %w", err)
	}

	// send batch
	w.log.InfoContext(ctx, fmt.Sprintf("submitting dc batch of %d dust transfers", len(dcBatch.Submissions())))
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send dust transfer transactions: %w", err)
	}
	proofs, err := w.extractProofsFromBatch(dcBatch)
	if err != nil {
		return nil, fmt.Errorf("failed to extract proofs from dc batch: %w", err)
	}

	// send swap tx, return swap proof
	swapProof, err := w.swapDCBills(ctx, k, proofs, lockedTargetUnit)
	if err != nil {
		return nil, err
	}
	return &DustCollectionResult{SwapProof: swapProof, FeeSum: swapProof.TxRecord.ServerMetadata.ActualFee + w.sumPaidFees(proofs)}, nil
}

func (w *DustCollector) swapDCBills(ctx context.Context, k *account.AccountKey, dcProofs []*wallet.Proof, lockedTargetUnit *unitlock.LockedUnit) (*wallet.Proof, error) {
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}

	// build tx
	swapTx, err := tx_builder.NewSwapTx(k, w.systemID, dcProofs, lockedTargetUnit.UnitID, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to build swap tx: %w", err)
	}

	// create new batch for sending tx
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend, w.log)
	sub := &txsubmitter.TxSubmission{
		UnitID:      swapTx.UnitID(),
		TxHash:      swapTx.Hash(crypto.SHA256),
		Transaction: swapTx,
	}
	dcBatch.Add(sub)

	// update locked bill with swap tx
	lockedTargetUnit.Transactions = []*unitlock.Transaction{unitlock.NewTransaction(sub.Transaction)}
	err = w.unitLocker.LockUnit(lockedTargetUnit)
	if err != nil {
		return nil, fmt.Errorf("failed to lock unit for swap tx: %w", err)
	}

	// send tx
	w.log.InfoContext(ctx, fmt.Sprintf("sending swap tx with timeout=%d", timeout), logger.UnitID(lockedTargetUnit.UnitID))
	err = dcBatch.SendTx(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to send swap tx: %w", err)
	}
	if err := w.unitLocker.UnlockUnit(k.PubKey, lockedTargetUnit.UnitID); err != nil {
		return nil, fmt.Errorf("failed to unlock unit: %w", err)
	}
	return sub.Proof, nil
}

func (w *DustCollector) extractProofsFromBatch(dcBatch *txsubmitter.TxSubmissionBatch) ([]*wallet.Proof, error) {
	var proofs []*wallet.Proof
	for _, sub := range dcBatch.Submissions() {
		proofs = append(proofs, sub.Proof)
	}
	return proofs, nil
}

func (w *DustCollector) getTxTimeout(ctx context.Context) (uint64, error) {
	roundNr, err := w.backend.GetRoundNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch round number: %w", err)
	}
	return roundNr + w.txTimeout, nil
}

func (w *DustCollector) getBillByID(bills []*wallet.Bill, id types.UnitID) *wallet.Bill {
	for _, b := range bills {
		if bytes.Equal(b.Id, id) {
			return b
		}
	}
	return nil
}

func (w *DustCollector) getLockedTargetBill(accountID []byte) (*unitlock.LockedUnit, error) {
	units, err := w.unitLocker.GetUnits(accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to load locked units: %w", err)
	}
	for _, unit := range units {
		if unit.LockReason == unitlock.LockReasonCollectDust {
			return unit, nil
		}
	}
	return nil, nil
}

// verifyLockedUnitValid returns true if the wallet contains the locked unit, false otherwise.
// Alternatively, backend could have GetBill(ID) endpoint which would be a cleaner way to achieve the same result.
func (w *DustCollector) verifyLockedUnitValid(ctx context.Context, accountKey *account.AccountKey, lockedBill *unitlock.LockedUnit) (bool, error) {
	bills, err := w.backend.GetBills(ctx, accountKey.PubKey)
	if err != nil {
		return false, fmt.Errorf("failed to fetch bills: %w", err)
	}
	actualLockedBill := w.getBillByID(bills, lockedBill.UnitID)
	if actualLockedBill != nil {
		if bytes.Equal(actualLockedBill.TxHash, lockedBill.TxHash) {
			return true, nil
		}
	}
	return false, nil
}

func (w *DustCollector) sumPaidFees(proofs []*wallet.Proof) uint64 {
	sum := uint64(0)
	for _, proof := range proofs {
		sum += proof.TxRecord.ServerMetadata.ActualFee
	}
	return sum
}
