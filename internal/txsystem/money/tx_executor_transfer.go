package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
)

var (
	ErrInvalidDataType  = errors.New("invalid data type")
	ErrInvalidBillValue = errors.New("transaction value must be equal to bill value")
)

func handleTransferTx(s *state.State, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[TransferAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		if err := validateTransferTx(tx, attr, s); err != nil {
			return nil, fmt.Errorf("invalid transfer tx: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()
		// update state
		updateDataFunc := updateBillDataFunc(tx, currentBlockNumber, hashAlgorithm)
		setOwnerFunc := state.SetOwner(tx.UnitID(), attr.NewBearer)
		if err := s.Apply(
			setOwnerFunc,
			updateDataFunc,
		); err != nil {
			return nil, fmt.Errorf("transfer: failed to update state: %w", err)
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateTransferTx(tx *types.TransactionOrder, attr *TransferAttributes, s *state.State) error {
	data, err := s.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	return validateTransfer(data.Data(), attr)
}

func validateTransfer(data state.UnitData, attr *TransferAttributes) error {
	return validateAnyTransfer(data, attr.Backlink, attr.TargetValue)
}

func validateAnyTransfer(data state.UnitData, backlink []byte, targetValue uint64) error {
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

func updateBillDataFunc(tx *types.TransactionOrder, currentBlockNumber uint64, hashAlgorithm crypto.Hash) state.Action {
	unitID := tx.UnitID()
	return state.UpdateUnitData(unitID,
		func(data state.UnitData) (state.UnitData, error) {
			bd, ok := data.(*BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			bd.T = currentBlockNumber
			bd.Backlink = tx.Hash(hashAlgorithm)
			return bd, nil
		})
}
