package money

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

func handleTransferDCTx(state *rma.Tree, dustCollector *DustCollector, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[TransferDCAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferDCAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing transferDC %v", tx)
		if err := validateTransferDCTx(tx, attr, state); err != nil {
			return nil, fmt.Errorf("invalid transferDC transaction: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		decrCreditFunc := fc.DecrCredit(fcrID, fee, tx.Hash(hashAlgorithm))
		updateDataFunc := updateBillDataFunc(tx, currentBlockNumber, hashAlgorithm)
		unitID := util.BytesToUint256(tx.UnitID())
		setOwnerFunc := rma.SetOwner(unitID, dustCollectorPredicate, tx.Hash(hashAlgorithm))
		if err := state.AtomicUpdate(
			decrCreditFunc,
			updateDataFunc,
			setOwnerFunc,
		); err != nil {
			return nil, fmt.Errorf("transferDC: failed to update state: %w", err)
		}

		// record dust bills for later deletion
		dustCollector.AddDustBill(unitID, currentBlockNumber)
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateTransferDCTx(tx *types.TransactionOrder, attr *TransferDCAttributes, state *rma.Tree) error {
	data, err := state.GetUnit(util.BytesToUint256(tx.UnitID()))
	if err != nil {
		return err
	}
	return validateTransferDC(data.Data, attr)
}

func validateTransferDC(data rma.UnitData, tx *TransferDCAttributes) error {
	return validateAnyTransfer(data, tx.Backlink, tx.TargetValue)
}
