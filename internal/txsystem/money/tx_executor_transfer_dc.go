package money

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

func handleTransferDCTx(state *rma.Tree, dustCollector *DustCollector, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[*transferDCWrapper] {
	return func(tx *transferDCWrapper, currentBlockNumber uint64) error {
		log.Debug("Processing transferDC %v", tx.transaction.ToLogString(log))

		if err := validateTransferDCTx(tx, state); err != nil {
			return fmt.Errorf("invalid transferDC transaction: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})
		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		decrCreditFunc := fc.DecrCredit(fcrID, tx.transaction.ServerMetadata.Fee, tx.Hash(hashAlgorithm))
		updateDataFunc := updateBillDataFunc(tx, currentBlockNumber, hashAlgorithm)
		setOwnerFunc := rma.SetOwner(tx.UnitID(), dustCollectorPredicate, tx.Hash(hashAlgorithm))
		if err := state.AtomicUpdate(
			decrCreditFunc,
			updateDataFunc,
			setOwnerFunc,
		); err != nil {
			return fmt.Errorf("transferDC: failed to update state: %w", err)
		}

		// record dust bills for later deletion
		dustCollector.AddDustBill(tx.UnitID(), currentBlockNumber)
		return nil
	}
}

func validateTransferDCTx(tx *transferDCWrapper, state *rma.Tree) error {
	data, err := state.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateTransferDC(data.Data, tx)
}

func validateTransferDC(data rma.UnitData, tx *transferDCWrapper) error {
	return validateAnyTransfer(data, tx.Backlink(), tx.TargetValue())
}
