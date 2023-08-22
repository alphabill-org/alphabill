package money

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
)

type (
	DustCollector struct {
		systemID      []byte
		maxBillsPerDC int
		backend       BackendAPI
		unitLocker    UnitLocker
	}

	UnitLocker interface {
		GetUnits() ([]*unitlock.LockedUnit, error)
		GetUnit(unitID []byte) (*unitlock.LockedUnit, error)
		LockUnit(lockedBill *unitlock.LockedUnit) error
		UnlockUnit(unitID []byte) error
		Close() error
	}
)

func NewDustCollector(systemID []byte, maxBillsPerDC int, backend BackendAPI, unitLocker UnitLocker) *DustCollector {
	return &DustCollector{
		systemID:      systemID,
		maxBillsPerDC: maxBillsPerDC,
		backend:       backend,
		unitLocker:    unitLocker,
	}
}

// CollectDust joins up to N units into existing target unit, prioritizing small units first. The largest unit is
// selected as the target unit. Returns swap transaction proof or error or nil if there's not enough bills to swap.
func (w *DustCollector) CollectDust(ctx context.Context, accountKey *account.AccountKey) (*wallet.Proof, error) {
	proof, err := w.runExistingDustCollection(ctx, accountKey)
	if err != nil {
		return nil, fmt.Errorf("failed to run existing swap: %w", err)
	}
	if proof != nil {
		return proof, nil
	}
	return w.runDustCollection(ctx, accountKey)
}

// runExistingDustCollection executes dust collection process using existing locked bill, if one exists
func (w *DustCollector) runExistingDustCollection(ctx context.Context, accountKey *account.AccountKey) (*wallet.Proof, error) {
	lockedTargetBill, err := w.getLockedTargetBill()
	if err != nil {
		return nil, err
	}
	if lockedTargetBill == nil {
		return nil, nil
	}
	log.Info("locked dc unit found for unit=", lockedTargetBill.UnitID)

	// verify locked unit not confirmed i.e. swap not already completed
	for _, tx := range lockedTargetBill.Transactions {
		if tx.TxOrder.PayloadType() == money.PayloadTypeSwapDC {
			proof, err := w.backend.GetTxProof(ctx, tx.TxOrder.UnitID(), tx.TxHash)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch proof: %w", err)
			}
			if proof != nil {
				// if it's confirmed unlock the unit and return swap proof
				if err := w.unitLocker.UnlockUnit(lockedTargetBill.UnitID); err != nil {
					return nil, fmt.Errorf("failed to unlock unit: %w", err)
				}
				return proof, nil
			}
		}
	}

	// verify locked unit still usable
	valid, err := w.verifyLockedUnitValid(ctx, accountKey, lockedTargetBill)
	if err != nil {
		return nil, err
	}
	if !valid {
		log.Warning("locked unit no longer valid, unlocking the unit")
		if err := w.unitLocker.UnlockUnit(lockedTargetBill.UnitID); err != nil {
			return nil, fmt.Errorf("failed to unlock unit: %w", err)
		}
		return nil, nil
	}
	log.Info("locked unit still valid")

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
		return w.swapDCBills(ctx, accountKey, proofs, lockedTargetBill)
	}
	// if no proofs found, run normal DC
	log.Info("no dust txs confirmed, unlocking target unit")
	if err := w.unitLocker.UnlockUnit(lockedTargetBill.UnitID); err != nil {
		return nil, fmt.Errorf("failed to unlock unit: %w", err)
	}
	return nil, nil
}

func (w *DustCollector) runDustCollection(ctx context.Context, accountKey *account.AccountKey) (*wallet.Proof, error) {
	// fetch non-dc bills
	bills, err := w.backend.GetBills(ctx, accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	bills, err = w.filterLockedBills(bills)
	if err != nil {
		return nil, err
	}
	if len(bills) < 2 {
		log.Info("account has less than two unlocked bills, skipping dust collection")
		return nil, nil
	}
	// sort bills by value smallest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value < bills[j].Value
	})
	// use the largest bill as target
	targetBill := bills[len(bills)-1]
	billCountToSwap := util.Min(w.maxBillsPerDC, len(bills)-1)
	return w.submitDCBatch(ctx, accountKey, targetBill, bills[:billCountToSwap])
}

func (w *DustCollector) filterLockedBills(bills []*wallet.Bill) ([]*wallet.Bill, error) {
	var nonLockedBills []*wallet.Bill
	for _, b := range bills {
		exists, err := w.unitLocker.GetUnit(b.GetID())
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
	billsResponse, err := w.backend.ListBills(ctx, k.PubKey, true)
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

func (w *DustCollector) submitDCBatch(ctx context.Context, k *account.AccountKey, targetBill *wallet.Bill, billsToSwap []*wallet.Bill) (*wallet.Proof, error) {
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}
	fcb, err := w.backend.GetFeeCreditBill(ctx, k.PubKeyHash.Sha256)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	txsCost := tx_builder.MaxFee * uint64(len(billsToSwap)+1) // +1 for swap
	if fcb.GetValue() < txsCost {
		return nil, fmt.Errorf("insufficient fee credit balance for transactions: need at least %d Tema "+
			"but have %d Tema to send swap and %d dust transfer transactions", txsCost, fcb.GetValue(), len(billsToSwap))
	}
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend)
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
	lockedTargetUnit := unitlock.NewLockedUnit(targetBill.Id, targetBill.TxHash, unitlock.LockReasonCollectDust, lockedUnitTxs...)
	err = w.unitLocker.LockUnit(lockedTargetUnit)
	if err != nil {
		return nil, fmt.Errorf("failed to lock unit for dc batch: %w", err)
	}

	// send batch
	log.Info("submitting dc batch of ", len(dcBatch.Submissions()), " dust transfers")
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send dust transfer transactions: %w", err)
	}
	proofs, err := w.extractProofsFromBatch(dcBatch)
	if err != nil {
		return nil, fmt.Errorf("failed to extract proofs from dc batch: %w", err)
	}

	// send swap tx, return swap proof
	return w.swapDCBills(ctx, k, proofs, lockedTargetUnit)
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
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend)
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
	log.Info(fmt.Sprintf("sending swap tx: targetUnitID=%s timeout=%d", hexutil.Encode(lockedTargetUnit.UnitID), timeout))
	err = dcBatch.SendTx(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to send swap tx: %w", err)
	}
	if err := w.unitLocker.UnlockUnit(lockedTargetUnit.UnitID); err != nil {
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
	return roundNr + txTimeoutBlockCount, nil
}

func (w *DustCollector) getBillByID(bills []*wallet.Bill, id wallet.UnitID) *wallet.Bill {
	for _, b := range bills {
		if bytes.Equal(b.Id, id) {
			return b
		}
	}
	return nil
}

func (w *DustCollector) getLockedTargetBill() (*unitlock.LockedUnit, error) {
	units, err := w.unitLocker.GetUnits()
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
