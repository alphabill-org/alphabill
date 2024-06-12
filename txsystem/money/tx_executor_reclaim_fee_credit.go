package money

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrReclaimFCInvalidTargetUnit        = errors.New("invalid target unit")
	ErrReclaimFCInvalidTxFee             = errors.New("the transaction fees cannot exceed the transferred value")
	ErrReclaimFCInvalidTargetUnitCounter = errors.New("invalid target unit counter")
)

func (m *Module) executeReclaimFCTx(tx *types.TransactionOrder, attr *fc.ReclaimFeeCreditAttributes, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	// calculate actual tx fee cost
	fee := exeCtx.CalculateCost()
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
		newBillData.T = exeCtx.CurrentRound()
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

func (m *Module) validateReclaimFCTx(tx *types.TransactionOrder, attr *fc.ReclaimFeeCreditAttributes, execCtx txtypes.ExecutionContext) error {
	unitID := tx.UnitID()
	unit, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("get unit error: %w", err)
	}
	bd, ok := unit.Data().(*money.BillData)
	if !ok {
		return errors.New("invalid unit type")
	}
	if tx.GetClientFeeCreditRecordID() != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}
	closeFeeCreditTx := attr.CloseFeeCreditTransfer
	closeFCAttr := &fc.CloseFeeCreditAttributes{}
	if err = closeFeeCreditTx.TransactionOrder.UnmarshalAttributes(closeFCAttr); err != nil {
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
	// verify predicate
	if err = m.execPredicate(unit.Bearer(), tx.OwnerProof, tx, execCtx); err != nil {
		return err
	}
	// verify proof
	if err = types.VerifyTxProof(attr.CloseFeeCreditProof, attr.CloseFeeCreditTransfer, m.trustBase, m.hashAlgorithm); err != nil {
		return fmt.Errorf("invalid proof: %w", err)
	}
	return nil
}
