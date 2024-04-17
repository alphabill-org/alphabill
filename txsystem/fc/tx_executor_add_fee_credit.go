package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
)

func handleAddFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[fc.AddFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
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
			fcr := &fc.FeeCreditRecord{
				Balance:  v,
				Backlink: txHash,
				Timeout:  transferFc.LatestAdditionTime + 1,
				Locked:   0,
			}
			updateFunc = unit.AddCredit(unitID, attr.FeeCreditOwnerCondition, fcr)
		} else {
			// increment credit
			updateFunc = unit.IncrCredit(unitID, v, transferFc.LatestAdditionTime+1, txHash)
		}

		if err = f.state.Apply(updateFunc); err != nil {
			return nil, fmt.Errorf("addFC state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func getTransferPayloadAttributes(transfer *types.TransactionRecord) (*fc.TransferFeeCreditAttributes, error) {
	transferPayload := &fc.TransferFeeCreditAttributes{}
	if err := transfer.TransactionOrder.UnmarshalAttributes(transferPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferPayload, nil
}
