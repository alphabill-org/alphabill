package money

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

type (
	dustCollectorTransfer struct {
		id         types.UnitID
		tx         *types.TransactionRecord
		attributes *money.TransferDCAttributes
	}
)

func (m *Module) executeSwapTx(tx *types.TransactionOrder, attr *money.SwapDCAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
	// reduce dc-money supply by target value and update timeout and backlink
	updateDCMoneySupplyFn := state.UpdateUnitData(DustCollectorMoneySupplyID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", DustCollectorMoneySupplyID)
			}
			bd.V -= attr.TargetValue
			bd.T = exeCtx.CurrentBlockNumber
			bd.Counter += 1
			return bd, nil
		},
	)
	// increase target unit value by swap amount
	updateTargetUnitFn := state.UpdateUnitData(tx.UnitID(),
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", tx.UnitID())
			}
			bd.V += attr.TargetValue
			bd.T = exeCtx.CurrentBlockNumber
			bd.Counter += 1
			bd.Locked = 0
			return bd, nil
		})
	if err := m.state.Apply(updateDCMoneySupplyFn, updateTargetUnitFn); err != nil {
		return nil, fmt.Errorf("unit update failed: %w", err)
	}
	return &types.ServerMetadata{
		ActualFee:        m.feeCalculator(),
		TargetUnits:      []types.UnitID{tx.UnitID(), DustCollectorMoneySupplyID},
		SuccessIndicator: types.TxStatusSuccessful,
	}, nil
}

func (m *Module) validateSwapTx(tx *types.TransactionOrder, attr *money.SwapDCAttributes, exeCtx *txsystem.TxExecutionContext) error {
	// 2. there is sufficient DC-money supply
	dcMoneySupply, err := m.state.GetUnit(DustCollectorMoneySupplyID, false)
	if err != nil {
		return fmt.Errorf("DC-money supply unit error: %w", err)
	}
	dcMoneySupplyBill, ok := dcMoneySupply.Data().(*money.BillData)
	if !ok {
		return errors.New("DC-money supply invalid data type")
	}
	if dcMoneySupplyBill.V < attr.TargetValue {
		return errors.New("insufficient DC-money supply")
	}
	// 3. tx unit id identifies an existing bill
	unitData, err := m.state.GetUnit(tx.UnitID(), false)
	if err != nil {
		return fmt.Errorf("target unit error: %w", err)
	}
	// if unit exists (get returns no error) it must not be nil, if unit data is nil then this type assertion will fail
	billData, ok := unitData.Data().(*money.BillData)
	if !ok {
		return fmt.Errorf("target unit invalid data type")
	}
	// 5. bills were transferred to DC
	dustTransfers, err := getDCTransfers(attr)
	if err != nil {
		return fmt.Errorf("failed to extract DC transfers: %w", err)
	}
	// 1. target value is the sum of the values of the transDC payments
	sum := sumDcTransferValues(dustTransfers)
	if attr.TargetValue != sum {
		return fmt.Errorf("target value must be equal to the sum of dust transfer values: expected %d vs provided %d", sum, attr.TargetValue)
	}
	if len(dustTransfers) != len(attr.DcTransferProofs) {
		return fmt.Errorf("invalid count of proofs: expected %d vs provided %d", len(dustTransfers), len(attr.DcTransferProofs))
	}
	if err = m.execPredicate(unitData.Bearer(), tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("swap tx predicate validation failed: %w", err)
	}
	for i, dcTx := range dustTransfers {
		// 4. transfers were in the money partition
		if dcTx.tx.TransactionOrder.SystemID() != m.systemID {
			return fmt.Errorf("dust transfer system id is not money partition system id: expected %s vs provided %s",
				m.systemID, dcTx.tx.TransactionOrder.SystemID())
		}
		// 6. transfer orders are listed in strictly increasing order of bill identifiers
		// (this ensures that no source bill can be included multiple times
		if i > 0 && bytes.Compare(dcTx.id, dustTransfers[i-1].id) != 1 {
			return errors.New("dust transfer orders are not listed in strictly increasing order of bill identifiers")
		}
		// 7. bill transfer orders contain correct target unit ids
		if !bytes.Equal(dcTx.attributes.TargetUnitID, tx.UnitID()) {
			return errors.New("dust transfer order target unit id is not equal to swap tx unit id")
		}
		// 8. bill transfer orders contain correct target counter values
		if dcTx.attributes.TargetUnitCounter != billData.Counter {
			return fmt.Errorf("dust transfer target counter is not equal to target unit counter: "+
				"expected %X vs provided %X", billData.Counter, dcTx.attributes.TargetUnitCounter)
		}
		// 9. transaction proofs of the bill transfer orders verify
		if err = types.VerifyTxProof(attr.DcTransferProofs[i], dcTx.tx, m.trustBase, m.hashAlgorithm); err != nil {
			return fmt.Errorf("proof is not valid: %w", err)
		}
	}
	return nil
}

func getDCTransfers(attr *money.SwapDCAttributes) ([]*dustCollectorTransfer, error) {
	if len(attr.DcTransfers) == 0 {
		return nil, errors.New("tx does not contain any dust transfers")
	}
	transfers := make([]*dustCollectorTransfer, len(attr.DcTransfers))
	for i, t := range attr.DcTransfers {
		if t == nil {
			return nil, fmt.Errorf("dc tx is nil: %d", i)
		}
		a := &money.TransferDCAttributes{}
		if t.TransactionOrder.PayloadType() != money.PayloadTypeTransDC {
			return nil, fmt.Errorf("invalid transfer DC payload type: %s", t.TransactionOrder.PayloadType())
		}
		if err := t.TransactionOrder.UnmarshalAttributes(a); err != nil {
			return nil, fmt.Errorf("invalid DC transfer: %w", err)
		}
		transfers[i] = &dustCollectorTransfer{
			id:         t.TransactionOrder.UnitID(),
			tx:         t,
			attributes: a,
		}
	}
	return transfers, nil
}

func sumDcTransferValues(txs []*dustCollectorTransfer) uint64 {
	var sum uint64
	for _, dcTx := range txs {
		sum += dcTx.attributes.Value
	}
	return sum
}
