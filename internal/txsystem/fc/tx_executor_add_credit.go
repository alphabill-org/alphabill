package fc

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

func handleAddFeeCreditTx(f *FeeCredit) txsystem.GenericExecuteFunc[transactions.AddFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.AddFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		f.logger.Debug("Processing addFC %v", tx)
		unitID := util.BytesToUint256(tx.UnitID())
		bd, _ := f.state.GetUnit(unitID)
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
		v := transferFc.Amount - fee

		var updateFunc rma.Action
		if bd == nil {
			// add credit
			fcr := &FeeCreditRecord{
				Balance: v,
				Hash:    tx.Hash(f.hashAlgorithm),
				Timeout: transferFc.LatestAdditionTime + 1,
			}
			updateFunc = AddCredit(unitID, attr.FeeCreditOwnerCondition, fcr, tx.Hash(f.hashAlgorithm))
		} else {
			// increment credit
			updateFunc = IncrCredit(unitID, v, transferFc.LatestAdditionTime+1, tx.Hash(f.hashAlgorithm))
		}

		if err := f.state.AtomicUpdate(updateFunc); err != nil {
			return nil, fmt.Errorf("addFC state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func getTransferPayloadAttributes(transfer *types.TransactionRecord) (*transactions.TransferFeeCreditAttributes, error) {
	transferPayload := &transactions.TransferFeeCreditAttributes{}
	if err := transfer.TransactionOrder.UnmarshalAttributes(transferPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferPayload, nil
}
