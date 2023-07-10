package money

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
)

func handleTransferDCTx(s *state.State, dustCollector *DustCollector, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[TransferDCAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferDCAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing transferDC %v", tx)
		if err := validateTransferDCTx(tx, attr, s); err != nil {
			return nil, fmt.Errorf("invalid transferDC tx: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		unitID := tx.UnitID()
		// update state
		updateDataFunc := updateBillDataFunc(tx, currentBlockNumber, hashAlgorithm)

		setOwnerFunc := state.SetOwner(unitID, dustCollectorPredicate)
		if err := s.Apply(
			updateDataFunc,
			setOwnerFunc,
		); err != nil {
			return nil, fmt.Errorf("transferDC: failed to update state: %w", err)
		}

		// record dust bills for later deletion
		dustCollector.AddDustBill(unitID, currentBlockNumber)
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}}, nil
	}
}

func validateTransferDCTx(tx *types.TransactionOrder, attr *TransferDCAttributes, s *state.State) error {
	data, err := s.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	return validateTransferDC(data.Data(), attr)
}

func validateTransferDC(data state.UnitData, tx *TransferDCAttributes) error {
	return validateAnyTransfer(data, tx.Backlink, tx.TargetValue)
}
