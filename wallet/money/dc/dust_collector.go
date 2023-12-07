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

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wallet"
	"github.com/alphabill-org/alphabill/wallet/account"
	"github.com/alphabill-org/alphabill/wallet/money/backend"
	"github.com/alphabill-org/alphabill/wallet/money/tx_builder"
	"github.com/alphabill-org/alphabill/wallet/txsubmitter"
	"github.com/alphabill-org/alphabill/wallet/unitlock"
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
		SwapProof *wallet.Proof
		LockProof *wallet.Proof
	}

	BackendAPI interface {
		GetBalance(ctx context.Context, pubKey []byte, includeDCBills bool) (uint64, error)
		ListBills(ctx context.Context, pubKey []byte, includeDCBills bool, offsetKey string, limit int) (*backend.ListBillsResponse, error)
		GetBills(ctx context.Context, pubKey []byte) ([]*wallet.Bill, error)
		GetRoundNumber(ctx context.Context) (*wallet.RoundNumber, error)
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
	return w.runDustCollection(ctx, accountKey, nil)
}

// GetFeeSum sums spent fees from the result
func (d *DustCollectionResult) GetFeeSum() (uint64, error) {
	if d == nil {
		return 0, nil
	}
	var feeSum uint64
	if d.SwapProof != nil {
		feeSum += d.SwapProof.TxRecord.GetActualFee()
		var swapAttr *money.SwapDCAttributes
		if err := d.SwapProof.TxRecord.TransactionOrder.UnmarshalAttributes(&swapAttr); err != nil {
			return 0, fmt.Errorf("failed to unmarshal swap transaction to calculate fee sum: %w", err)
		}
		for _, dcTx := range swapAttr.DcTransfers {
			feeSum += dcTx.GetActualFee()
		}
	}
	if d.LockProof != nil {
		feeSum += d.LockProof.TxRecord.GetActualFee()
	}
	return feeSum, nil
}

/*
Executes dust collection process using existing locked bill, if one exists. More specifically:
 1. wait for pending txs to either confirm or timeout
 2. verify server side bill still valid
 3. handle special cases depending on tx type
    3.1 swap tx => if confirmed return result
    3.2 lock tx => if confirmed then run normal dc with the locked unit
 4. swap confirmed dc txs and retry swap
*/
func (w *DustCollector) runExistingDustCollection(ctx context.Context, accountKey *account.AccountKey) (*DustCollectionResult, error) {
	targetBill, err := w.getLockedTargetBill(accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	if targetBill == nil {
		return nil, nil
	}
	w.log.InfoContext(ctx, fmt.Sprintf("locked target unit found, pubkey=%x", accountKey.PubKey), logger.UnitID(targetBill.UnitID))

	serverLockedBill, err := w.fetchLockedBill(ctx, accountKey, targetBill)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch server side locked bill: %w", err)
	}

	var dcProofs []*wallet.Proof
	for _, tx := range targetBill.Transactions {
		proof, err := w.waitForConf(ctx, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to wait for tx confirmation: %w", err)
		}
		if proof == nil {
			continue
		}
		payloadType := tx.TxOrder.PayloadType()
		switch payloadType {
		case money.PayloadTypeSwapDC:
			w.log.InfoContext(ctx, fmt.Sprintf("swap tx confirmed, returning swap proof, pubkey=%x", accountKey.PubKey), logger.UnitID(targetBill.UnitID))
			if err := w.unitLocker.UnlockUnit(accountKey.PubKey, targetBill.UnitID); err != nil {
				return nil, fmt.Errorf("failed to unlock unit: %w", err)
			}
			return &DustCollectionResult{SwapProof: proof}, nil
		case money.PayloadTypeLock:
			if serverLockedBill != nil {
				w.log.InfoContext(ctx, fmt.Sprintf("lock tx confirmed and target bill still valid, starting dc process with the existing target bill, pubkey=%x", accountKey.PubKey), logger.UnitID(targetBill.UnitID))
				return w.runDustCollection(ctx, accountKey, targetBill)
			}
			w.log.InfoContext(ctx, fmt.Sprintf("lock tx confirmed and target bill no longer valid, unlocking the unit and starting fresh dc process, pubkey=%x", accountKey.PubKey), logger.UnitID(targetBill.UnitID))
			if err := w.unitLocker.UnlockUnit(accountKey.PubKey, targetBill.UnitID); err != nil {
				return nil, fmt.Errorf("failed to unlock unit: %w", err)
			}
			return w.runDustCollection(ctx, accountKey, nil)
		case money.PayloadTypeTransDC:
			dcProofs = append(dcProofs, proof)
		}
	}
	// verify server side bill is still valid
	// it may be that server side bill was manually locked or spent etc
	if serverLockedBill == nil {
		w.log.InfoContext(ctx, "server side locked unit not found, unlocking target unit", logger.UnitID(targetBill.UnitID))
		if err := w.unitLocker.UnlockUnit(accountKey.PubKey, targetBill.UnitID); err != nil {
			return nil, fmt.Errorf("failed to unlock unit: %w", err)
		}
		return w.runDustCollection(ctx, accountKey, nil)
	}
	// if no dc proofs found then attempt to fetch them by target bill e.g. in case we have failed swap transfer
	if len(dcProofs) == 0 {
		dcProofs, err = w.fetchDCProofsForTargetUnit(ctx, accountKey, targetBill)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch dc proofs for target bill: %x", targetBill.UnitID)
		}
	}
	// if dc proofs found, swap them
	if len(dcProofs) > 0 {
		return w.swapDCBills(ctx, accountKey, dcProofs, targetBill)
	}
	// if no dc proofs found, retry dc process with the locked bill
	return w.runDustCollection(ctx, accountKey, targetBill)
}

// runDustCollection executes dust collection process, if target bill is not provided then selects and locks target bill
func (w *DustCollector) runDustCollection(ctx context.Context, accountKey *account.AccountKey, lockedTargetBill *unitlock.LockedUnit) (*DustCollectionResult, error) {
	// fetch non-dc bills
	bills, err := w.backend.GetBills(ctx, accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	// filter any locked bills
	bills, err = w.filterLockedBills(bills, accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	// sort bills by value smallest first
	sort.Slice(bills, func(i, j int) bool {
		return bills[i].Value < bills[j].Value
	})
	// fetch fee credit bill
	fcb, err := w.backend.GetFeeCreditBill(ctx, money.NewFeeCreditRecordID(nil, accountKey.PubKeyHash.Sha256))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fee credit bill: %w", err)
	}
	var lockTxProof *wallet.Proof
	var billsToSwap []*wallet.Bill
	if lockedTargetBill == nil {
		// verify that we have at least two bills to join
		if len(bills) < 2 {
			w.log.InfoContext(ctx, "account has less than two unlocked bills, skipping dust collection")
			return nil, nil
		}
		// use the largest bill as target
		targetBill := bills[len(bills)-1]
		billCountToSwap := min(w.maxBillsPerDC, len(bills)-1)
		billsToSwap = bills[:billCountToSwap]
		txsCost := tx_builder.MaxFee * uint64(len(billsToSwap)+2) // +2 for swap and lock tx
		if fcb.GetValue() < txsCost {
			return nil, fmt.Errorf("insufficient fee credit balance for transactions: need at least %d Tema "+
				"but have %d Tema to send lock tx, %d dust transfer transactions and swap tx", txsCost, fcb.GetValue(), len(billsToSwap))
		}
		lockedTargetBill, lockTxProof, err = w.lockTargetBill(ctx, accountKey, targetBill)
		if err != nil {
			return nil, fmt.Errorf("failed to lock target bill: %w", err)
		}
	} else {
		// verify that we have at least one bill to join
		if len(bills) == 0 {
			w.log.InfoContext(ctx, "account has no unlocked bills, skipping dust collection")
			return nil, nil
		}
		billCountToSwap := min(w.maxBillsPerDC, len(bills))
		billsToSwap = bills[:billCountToSwap]
		txsCost := tx_builder.MaxFee * uint64(len(billsToSwap)+1) // +1 for swap tx
		if fcb.GetValue() < txsCost {
			return nil, fmt.Errorf("insufficient fee credit balance for transactions: need at least %d Tema "+
				"but have %d Tema to %d dust transfer transactions and swap tx", txsCost, fcb.GetValue(), len(billsToSwap))
		}
	}
	dcResult, err := w.submitDCBatch(ctx, accountKey, lockedTargetBill, billsToSwap)
	if err != nil {
		return nil, err
	}
	dcResult.LockProof = lockTxProof
	return dcResult, nil
}

// submitDCBatch creates dust transfers from given bills and target bill,
// the target bill is expected to be locked both locally and on server side.
func (w *DustCollector) submitDCBatch(ctx context.Context, k *account.AccountKey, targetBill *unitlock.LockedUnit, billsToSwap []*wallet.Bill) (*DustCollectionResult, error) {
	// create dc batch
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, err
	}
	dcBatch := txsubmitter.NewBatch(k.PubKey, w.backend, w.log)
	for _, b := range billsToSwap {
		tx, err := tx_builder.NewDustTx(k, w.systemID, &wallet.Bill{Id: b.GetID(), Value: b.Value, TxHash: b.TxHash}, targetBill.UnitID, targetBill.TxHash, timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to build dust transfer transaction: %w", err)
		}
		dcBatch.Add(&txsubmitter.TxSubmission{
			UnitID:      tx.UnitID(),
			TxHash:      tx.Hash(crypto.SHA256),
			Transaction: tx,
		})
	}

	// update locked bill with dc transfers
	var lockedUnitTxs []*unitlock.Transaction
	for _, sub := range dcBatch.Submissions() {
		lockedUnitTxs = append(lockedUnitTxs, unitlock.NewTransaction(sub.Transaction))
	}
	targetBill.Transactions = lockedUnitTxs
	if err := w.unitLocker.LockUnit(targetBill); err != nil {
		return nil, fmt.Errorf("failed to lock unit for dc batch: %w", err)
	}

	// send dc batch
	w.log.InfoContext(ctx, fmt.Sprintf("submitting dc batch of %d dust transfers with target bill %x", len(dcBatch.Submissions()), targetBill.UnitID))
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send dust transfer transactions: %w", err)
	}
	proofs, err := w.extractProofsFromBatch(dcBatch)
	if err != nil {
		return nil, fmt.Errorf("failed to extract proofs from dc batch: %w", err)
	}

	// send swap tx, return swap proof
	return w.swapDCBills(ctx, k, proofs, targetBill)
}

// swapDCBills creates swap transfer from given dcProofs and target bill, joining the dcBills into the target bill,
// the target bill is expected to be locked both locally and on server side.
func (w *DustCollector) swapDCBills(ctx context.Context, k *account.AccountKey, dcProofs []*wallet.Proof, lockedTargetUnit *unitlock.LockedUnit) (*DustCollectionResult, error) {
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
	if err := w.unitLocker.LockUnit(lockedTargetUnit); err != nil {
		return nil, fmt.Errorf("failed to lock unit for swap tx: %w", err)
	}

	// send tx
	w.log.InfoContext(ctx, fmt.Sprintf("sending swap tx with timeout=%d", timeout), logger.UnitID(lockedTargetUnit.UnitID))
	if err := dcBatch.SendTx(ctx, true); err != nil {
		return nil, fmt.Errorf("failed to send swap tx: %w", err)
	}
	if err := w.unitLocker.UnlockUnit(k.PubKey, lockedTargetUnit.UnitID); err != nil {
		return nil, fmt.Errorf("failed to unlock unit: %w", err)
	}
	return &DustCollectionResult{SwapProof: sub.Proof}, nil
}

// fetchLockedBill returns target *wallet.Bill if the wallet contains server side locked bill that matches the
// locally locked bill, nil otherwise.
func (w *DustCollector) fetchLockedBill(ctx context.Context, accountKey *account.AccountKey, lockedBill *unitlock.LockedUnit) (*wallet.Bill, error) {
	// fetch all bills, alternatively, backend could have GetBill(ID) endpoint which would be a cleaner way to achieve
	// the same result.
	bills, err := w.backend.GetBills(ctx, accountKey.PubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bills: %w", err)
	}
	actualLockedBill := w.getBillByID(bills, lockedBill.UnitID)
	if actualLockedBill != nil {
		if actualLockedBill.IsLocked() && bytes.Equal(actualLockedBill.TxHash, lockedBill.TxHash) {
			return actualLockedBill, nil
		}
	}
	return nil, nil
}

func (w *DustCollector) lockTargetBill(ctx context.Context, k *account.AccountKey, targetBill *wallet.Bill) (*unitlock.LockedUnit, *wallet.Proof, error) {
	// create lock tx
	timeout, err := w.getTxTimeout(ctx)
	if err != nil {
		return nil, nil, err
	}
	lockTx, err := tx_builder.NewLockTx(k, w.systemID, targetBill.Id, targetBill.TxHash, unitlock.LockReasonCollectDust, timeout)
	if err != nil {
		return nil, nil, err
	}
	// lock target bill locally
	lockedTargetBill := unitlock.NewLockedUnit(k.PubKey, targetBill.Id, targetBill.TxHash, w.systemID, unitlock.LockReasonCollectDust, unitlock.NewTransaction(lockTx))
	if err := w.unitLocker.LockUnit(lockedTargetBill); err != nil {
		return nil, nil, fmt.Errorf("failed to lock bill for dc batch: %w", err)
	}
	// lock target bill server side
	w.log.InfoContext(ctx, fmt.Sprintf("locking target bill in node %x", targetBill.Id))
	lockTxHash := lockTx.Hash(crypto.SHA256)
	lockTxBatch := txsubmitter.NewBatch(k.PubKey, w.backend, w.log)
	lockTxBatch.Add(&txsubmitter.TxSubmission{
		UnitID:      lockTx.UnitID(),
		TxHash:      lockTxHash,
		Transaction: lockTx,
	})
	if err := lockTxBatch.SendTx(ctx, true); err != nil {
		return nil, nil, fmt.Errorf("failed to send or confirm lock tx: %w", err)
	}
	// update local locked unit after successful lock tx
	lockedTargetBill.TxHash = lockTxHash
	if err := w.unitLocker.LockUnit(lockedTargetBill); err != nil {
		return nil, nil, fmt.Errorf("failed to update locked bill: %w", err)
	}
	return lockedTargetBill, lockTxBatch.Submissions()[0].Proof, nil
}

func (w *DustCollector) filterLockedBills(bills []*wallet.Bill, accountID []byte) ([]*wallet.Bill, error) {
	var nonLockedBills []*wallet.Bill
	for _, b := range bills {
		// filter server side locked bills
		if b.IsLocked() {
			continue
		}
		// filter client-side locked bills
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
	for {
		// fetch round number before proof to ensure that we cannot miss the proof
		rnr, err := w.backend.GetRoundNumber(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch round number: %w", err)
		}
		proof, err := w.backend.GetTxProof(ctx, tx.TxOrder.UnitID(), tx.TxHash)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tx proof: %w", err)
		}
		if proof != nil {
			return proof, nil
		}
		if rnr.LastIndexedRoundNumber >= tx.TxOrder.Timeout() {
			break
		}
		select {
		case <-time.After(time.Second):
		case <-ctx.Done():
			return nil, errors.New("context canceled")
		}
	}
	return nil, nil
}

func (w *DustCollector) extractProofsFromBatch(dcBatch *txsubmitter.TxSubmissionBatch) ([]*wallet.Proof, error) {
	var proofs []*wallet.Proof
	for _, sub := range dcBatch.Submissions() {
		proofs = append(proofs, sub.Proof)
	}
	return proofs, nil
}

func (w *DustCollector) getTxTimeout(ctx context.Context) (uint64, error) {
	rnr, err := w.backend.GetRoundNumber(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch round number: %w", err)
	}
	return rnr.RoundNumber + w.txTimeout, nil
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
