package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

var (
	ErrReclaimFCInvalidTargetUnit        = errors.New("invalid target unit")
	ErrReclaimFCInvalidTxFee             = errors.New("the transaction fees cannot exceed the transferred value")
	ErrReclaimFCInvalidTargetUnitCounter = errors.New("invalid target unit counter")
)

func (m *Module) handleReclaimFeeCreditTx() txsystem.GenericExecuteFunc[fc.ReclaimFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *fc.ReclaimFeeCreditAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		unitID := tx.UnitID()
		unit, _ := m.state.GetUnit(unitID, false)
		if unit == nil {
			return nil, errors.New("reclaimFC: unit not found")
		}
		if err := m.execPredicate(unit.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
			return nil, err
		}
		bdd, ok := unit.Data().(*money.BillData)
		if !ok {
			return nil, errors.New("reclaimFC: invalid unit type")
		}

		if err := validateReclaimFC(tx, attr, bdd, m.trustBase, m.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("reclaimFC: validation failed: %w", err)
		}

		// calculate actual tx fee cost
		fee := m.feeCalculator()

		closeFCAttr := &fc.CloseFeeCreditAttributes{}
		closeFeeCreditTransfer := attr.CloseFeeCreditTransfer
		if err := closeFeeCreditTransfer.TransactionOrder.UnmarshalAttributes(closeFCAttr); err != nil {
			return nil, fmt.Errorf("reclaimFC: failed to unmarshal close fee credit attributes: %w", err)
		}

		// add reclaimed value to source unit
		v := closeFCAttr.Amount - closeFeeCreditTransfer.ServerMetadata.ActualFee - fee
		updateFunc := func(data types.UnitData) (types.UnitData, error) {
			newBillData, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
			}
			newBillData.V += v
			newBillData.T = exeCtx.CurrentBlockNr
			newBillData.Counter += 1
			newBillData.Locked = 0
			return newBillData, nil
		}
		updateAction := state.UpdateUnitData(unitID, updateFunc)

		if err := m.state.Apply(updateAction); err != nil {
			return nil, fmt.Errorf("reclaimFC: failed to update state: %w", err)
		}
		m.feeCreditTxRecorder.recordReclaimFC(
			&reclaimFeeCreditTx{
				tx:                  tx,
				attr:                attr,
				closeFCTransferAttr: closeFCAttr,
				reclaimFee:          fee,
				closeFee:            closeFeeCreditTransfer.ServerMetadata.ActualFee,
			},
		)
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateReclaimFC(tx *types.TransactionOrder, attr *fc.ReclaimFeeCreditAttributes, bd *money.BillData, trustBase types.RootTrustBase, hashAlgorithm crypto.Hash) error {
	if tx == nil {
		return ErrTxNil
	}
	if bd == nil {
		return ErrBillNil
	}
	if tx.GetClientFeeCreditRecordID() != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}

	closeFeeCreditTx := attr.CloseFeeCreditTransfer
	closeFCAttr := &fc.CloseFeeCreditAttributes{}
	if err := closeFeeCreditTx.TransactionOrder.UnmarshalAttributes(closeFCAttr); err != nil {
		return fmt.Errorf("invalid close fee credit attributes: %w", err)
	}

	if !bytes.Equal(tx.UnitID(), closeFCAttr.TargetUnitID) {
		return ErrReclaimFCInvalidTargetUnit
	}
	if bd.Counter != closeFCAttr.TargetUnitCounter {
		return ErrReclaimFCInvalidTargetUnitCounter
	}
	if bd.Counter != attr.Counter {
		return ErrInvalidCounter
	}
	//
	if closeFeeCreditTx.ServerMetadata.ActualFee+tx.Payload.ClientMetadata.MaxTransactionFee > closeFCAttr.Amount {
		return ErrReclaimFCInvalidTxFee
	}
	// verify proof
	if err := types.VerifyTxProof(attr.CloseFeeCreditProof, attr.CloseFeeCreditTransfer, trustBase, hashAlgorithm); err != nil {
		return fmt.Errorf("invalid proof: %w", err)
	}
	return nil
}
