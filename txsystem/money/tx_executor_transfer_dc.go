package money

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	"github.com/alphabill-org/alphabill/validator/internal/types"
)

func handleTransferDCTx(s *state.State, dustCollector *DustCollector, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[TransferDCAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferDCAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		if err := validateTransferDCTx(tx, attr, s); err != nil {
			return nil, fmt.Errorf("invalid transferDC tx: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		unitID := tx.UnitID()

		// 1. SetOwner(ι, DC)
		setOwnerFn := state.SetOwner(unitID, dustCollectorPredicate)

		// 2. UpdateData(ι0, f′), where f′ : D.v → D.v + N[ι].D.v – increase DC money supply by N[ι].D.v
		updateDCMoneySupplyFn := state.UpdateUnitData(dustCollectorMoneySupplyID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", dustCollectorMoneySupplyID)
				}
				bd.V += attr.Value
				return bd, nil
			})

		// 3. UpdateData(ι, f), where f(D) = (0, S.n, H(P))
		updateUnitFn := state.UpdateUnitData(unitID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
				}
				bd.V = 0
				bd.T = currentBlockNumber
				bd.Backlink = tx.Hash(hashAlgorithm)
				return bd, nil
			})

		if err := s.Apply(
			setOwnerFn,
			updateDCMoneySupplyFn,
			updateUnitFn,
		); err != nil {
			return nil, fmt.Errorf("transferDC: failed to update state: %w", err)
		}

		// record dust bills for later deletion TODO AB-1133
		// dustCollector.AddDustBill(unitID, currentBlockNumber)
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
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
	return validateAnyTransfer(data, tx.Backlink, tx.Value)
}
