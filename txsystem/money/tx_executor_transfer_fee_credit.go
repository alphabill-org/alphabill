package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

var (
	ErrTxNil                       = errors.New("tx is nil")
	ErrTxAttrNil                   = errors.New("tx attributes is nil")
	ErrBillNil                     = errors.New("bill is nil")
	ErrBillLocked                  = errors.New("bill is already locked")
	ErrTargetSystemIdentifierEmpty = errors.New("TargetSystemIdentifier is empty")
	ErrTargetRecordIDEmpty         = errors.New("TargetRecordID is empty")
	ErrAdditionTimeInvalid         = errors.New("EarliestAdditionTime is greater than LatestAdditionTime")
	ErrRecordIDExists              = errors.New("fee tx cannot contain fee credit reference")
	ErrFeeProofExists              = errors.New("fee tx cannot contain fee authorization proof")
	ErrInvalidFCValue              = errors.New("the amount to transfer cannot exceed the value of the bill")
	ErrInvalidFeeValue             = errors.New("the transaction max fee cannot exceed the transferred amount")
	ErrInvalidCounter              = errors.New("the transaction counter is not equal to the unit counter")
)

func (m *Module) handleTransferFeeCreditTx() txsystem.GenericExecuteFunc[fc.TransferFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *fc.TransferFeeCreditAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()
		unit, _ := m.state.GetUnit(unitID, false)
		if unit == nil {
			return nil, fmt.Errorf("transferFC: unit not found %X", tx.UnitID())
		}
		if err := m.execPredicate(unit.Bearer(), tx.OwnerProof, tx); err != nil {
			return nil, fmt.Errorf("verify owner proof: %w", err)
		}
		billData, ok := unit.Data().(*money.BillData)
		if !ok {
			return nil, errors.New("transferFC: invalid unit type")
		}
		if err := validateTransferFC(tx, attr, billData); err != nil {
			return nil, fmt.Errorf("transferFC: validation failed: %w", err)
		}

		// remove value from source unit, zero value bills get removed later
		action := state.UpdateUnitData(unitID, func(data types.UnitData) (types.UnitData, error) {
			newBillData, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			newBillData.V -= attr.Amount
			newBillData.T = exeCtx.CurrentBlockNr
			newBillData.Counter += 1
			return newBillData, nil
		})
		if err := m.state.Apply(action); err != nil {
			return nil, fmt.Errorf("transferFC: failed to update state: %w", err)
		}

		fee := m.feeCalculator()

		// record fee tx for end of the round consolidation
		m.feeCreditTxRecorder.recordTransferFC(&transferFeeCreditTx{
			tx:   tx,
			attr: attr,
			fee:  fee,
		})
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func validateTransferFC(tx *types.TransactionOrder, attr *fc.TransferFeeCreditAttributes, bd *money.BillData) error {
	if tx == nil {
		return ErrTxNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if attr.TargetSystemIdentifier == 0 {
		return ErrTargetSystemIdentifierEmpty
	}
	if len(attr.TargetRecordID) == 0 {
		return ErrTargetRecordIDEmpty
	}
	if bd.IsLocked() {
		return ErrBillLocked
	}
	if attr.EarliestAdditionTime > attr.LatestAdditionTime {
		return ErrAdditionTimeInvalid
	}
	if uint64(attr.Amount) > bd.V {
		return ErrInvalidFCValue
	}
	if tx.Payload.ClientMetadata.MaxTransactionFee > attr.Amount {
		return ErrInvalidFeeValue
	}
	if bd.Counter != attr.Counter {
		return ErrInvalidCounter
	}
	if tx.GetClientFeeCreditRecordID() != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}
	return nil
}
