package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	fcunit "github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
)

func handleAddFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[transactions.AddFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.AddFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()

		bd, _ := f.state.GetUnit(unitID, false)
		if err := f.txValidator.ValidateAddFeeCredit(&AddFCValidationContext{
			Tx:                 tx,
			Unit:               bd,
			CurrentRoundNumber: currentBlockNumber,
		}); err != nil {
			return nil, fmt.Errorf("addFC tx validation failed: %w", err)
		}
		// calculate actual tx fee cost
		fee := f.feeCalculator()

		// find net value of credit
		transferFc, err := getTransferPayloadAttributes(attr.FeeCreditTransfer)
		if err != nil {
			return nil, err
		}

		v := transferFc.Amount - attr.FeeCreditTransfer.ServerMetadata.ActualFee - fee

		txHash := tx.Hash(f.hashAlgorithm)
		var updateFunc state.Action
		if bd == nil {
			// add credit
			fcr := &fcunit.FeeCreditRecord{
				Balance: v,
				Hash:    txHash,
				Timeout: transferFc.LatestAdditionTime + 1,
			}
			updateFunc = fcunit.AddCredit(unitID, attr.FeeCreditOwnerCondition, fcr)
		} else {
			// increment credit
			updateFunc = fcunit.IncrCredit(unitID, v, transferFc.LatestAdditionTime+1, txHash)
		}

		if err = f.state.Apply(updateFunc); err != nil {
			return nil, fmt.Errorf("addFC state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func getTransferPayloadAttributes(transfer *types.TransactionRecord) (*transactions.TransferFeeCreditAttributes, error) {
	transferPayload := &transactions.TransferFeeCreditAttributes{}
	if err := transfer.TransactionOrder.UnmarshalAttributes(transferPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferPayload, nil
}
