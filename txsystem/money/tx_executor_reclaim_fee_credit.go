package money

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrReclaimFCInvalidTargetUnit        = errors.New("invalid target unit")
	ErrReclaimFCInvalidTxFee             = errors.New("the transaction fees cannot exceed the transferred value")
	ErrReclaimFCInvalidTargetUnitCounter = errors.New("invalid target unit counter")
)

func (m *Module) executeReclaimFCTx(tx *types.TransactionOrder, attr *fc.ReclaimFeeCreditAttributes, _ *fc.ReclaimFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	fee := exeCtx.CalculateCost()

	// add reclaimed value to source unit
	reclaimAmount := util.BytesToUint64(exeCtx.GetData())
	v := reclaimAmount - fee
	updateFunc := func(data types.UnitData) (types.UnitData, error) {
		newBillData, ok := data.(*money.BillData)
		if !ok {
			return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
		}
		newBillData.Value += v
		newBillData.Counter += 1
		return newBillData, nil
	}
	updateAction := state.UpdateUnitData(unitID, updateFunc)

	if err := m.state.Apply(updateAction); err != nil {
		return nil, fmt.Errorf("reclaimFC: failed to update state: %w", err)
	}
	if err := m.feeCreditTxRecorder.recordReclaimFC(
		&reclaimFeeCreditTx{
			tx:            tx,
			attr:          attr,
			reclaimAmount: reclaimAmount,
			reclaimFee:    fee,
			closeFee:      attr.CloseFeeCreditProof.ActualFee(),
		},
	); err != nil {
		return nil, fmt.Errorf("failed to record reclaim fee credit transaction: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *Module) validateReclaimFCTx(tx *types.TransactionOrder, attr *fc.ReclaimFeeCreditAttributes, authProof *fc.ReclaimFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	unitID := tx.GetUnitID()
	unit, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("get unit error: %w", err)
	}
	bd, ok := unit.Data().(*money.BillData)
	if !ok {
		return errors.New("invalid unit type")
	}
	if tx.FeeCreditRecordID() != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}
	closeFcProof := attr.CloseFeeCreditProof
	if err = closeFcProof.IsValid(); err != nil {
		return fmt.Errorf("close fee credit proof is invalid: %w", err)
	}
	txo, err := closeFcProof.GetTransactionOrderV1()
	if err != nil {
		return fmt.Errorf("get transaction order error: %w", err)
	}
	closeFCAttr := &fc.CloseFeeCreditAttributes{}
	if err = txo.UnmarshalAttributes(closeFCAttr); err != nil {
		return fmt.Errorf("invalid close fee credit attributes: %w", err)
	}
	if m.pdr.NetworkID != txo.NetworkID {
		return fmt.Errorf("invalid network id: %d (expected %d)", txo.NetworkID, m.pdr.NetworkID)
	}
	if !bytes.Equal(tx.UnitID, closeFCAttr.TargetUnitID) {
		return ErrReclaimFCInvalidTargetUnit
	}
	if bd.Counter != closeFCAttr.TargetUnitCounter {
		return ErrReclaimFCInvalidTargetUnitCounter
	}
	feeLimit, ok := util.SafeAdd(tx.MaxFee(), closeFcProof.ActualFee())
	if !ok {
		return errors.New("failed to add Tx.MaxFee and CloseFC.ActualFee: overflow")
	}
	if closeFCAttr.Amount < feeLimit {
		return ErrReclaimFCInvalidTxFee
	}
	// verify predicate
	if err = m.execPredicate(bd.Owner(), authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return err
	}
	// verify proof
	if err = closeFcProof.Verify(m.orchestration.TrustBase); err != nil {
		return fmt.Errorf("invalid proof: %w", err)
	}
	// store reclaimed amount to execution context to not have to calculate it again later
	reclaimAmount := closeFCAttr.Amount - closeFcProof.ActualFee()
	exeCtx.SetData(util.Uint64ToBytes(reclaimAmount))
	return nil
}
