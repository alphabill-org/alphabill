package money

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

func handleTransferTx(state *rma.Tree, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[TransferAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing transfer %v", tx)
		if err := validateTransferTx(tx, attr, state); err != nil {
			return nil, fmt.Errorf("invalid transfer tx: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		decrCreditFunc := fc.DecrCredit(fcrID, fee, tx.Hash(hashAlgorithm))
		updateDataFunc := updateBillDataFunc(tx, currentBlockNumber, hashAlgorithm)
		setOwnerFunc := rma.SetOwner(util.BytesToUint256(tx.UnitID()), attr.NewBearer, tx.Hash(hashAlgorithm))
		if err := state.AtomicUpdate(
			decrCreditFunc,
			updateDataFunc,
			setOwnerFunc,
		); err != nil {
			return nil, fmt.Errorf("transfer: failed to update state: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateTransferTx(tx *types.TransactionOrder, attr *TransferAttributes, state *rma.Tree) error {
	data, err := state.GetUnit(util.BytesToUint256(tx.UnitID()))
	if err != nil {
		return err
	}
	return validateTransfer(data.Data, attr)
}

func validateTransfer(data rma.UnitData, attr *TransferAttributes) error {
	return validateAnyTransfer(data, attr.Backlink, attr.TargetValue)
}

func validateAnyTransfer(data rma.UnitData, backlink []byte, targetValue uint64) error {
	bd, ok := data.(*BillData)
	if !ok {
		return ErrInvalidDataType
	}
	if !bytes.Equal(backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}
	if targetValue != bd.V {
		return ErrInvalidBillValue
	}
	return nil
}

func updateBillDataFunc(tx *types.TransactionOrder, currentBlockNumber uint64, hashAlgorithm crypto.Hash) rma.Action {
	return rma.UpdateData(util.BytesToUint256(tx.UnitID()),
		func(data rma.UnitData) (newData rma.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				return data // TODO return error
			}
			bd.T = currentBlockNumber
			bd.Backlink = tx.Hash(hashAlgorithm)
			return bd
		}, tx.Hash(hashAlgorithm))
}
