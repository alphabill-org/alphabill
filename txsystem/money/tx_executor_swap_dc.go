package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

type (
	dustCollectorTransfer struct {
		id         types.UnitID
		tx         *types.TransactionRecord
		attributes *TransferDCAttributes
	}
	swapValidationContext struct {
		tx            *types.TransactionOrder
		attr          *SwapDCAttributes
		state         stateProvider
		systemID      types.SystemID
		hashAlgorithm crypto.Hash
		trustBase     map[string]abcrypto.Verifier
		execPredicate func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error
	}
	stateProvider interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
	}
)

func (m *Module) handleSwapDCTx() txsystem.GenericExecuteFunc[SwapDCAttributes] {
	return func(tx *types.TransactionOrder, attr *SwapDCAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		c := &swapValidationContext{
			tx:            tx,
			attr:          attr,
			state:         m.state,
			systemID:      m.systemID,
			hashAlgorithm: m.hashAlgorithm,
			trustBase:     m.trustBase,
			execPredicate: m.execPredicate,
		}
		if err := c.validateSwapTx(); err != nil {
			return nil, fmt.Errorf("invalid swap transaction: %w", err)
		}

		h := tx.Hash(m.hashAlgorithm)

		// reduce dc-money supply by target value and update timeout and backlink
		updateDCMoneySupplyFn := state.UpdateUnitData(DustCollectorMoneySupplyID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", DustCollectorMoneySupplyID)
				}
				bd.V -= attr.TargetValue
				bd.T = currentBlockNumber
				bd.Backlink = h
				return bd, nil
			},
		)
		// increase target unit value by swap amount
		updateTargetUnitFn := state.UpdateUnitData(tx.UnitID(),
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", tx.UnitID())
				}
				bd.V += attr.TargetValue
				bd.T = currentBlockNumber
				bd.Backlink = h
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
}

func (c *swapValidationContext) validateSwapTx() error {
	if err := c.isValid(); err != nil {
		return fmt.Errorf("swap validation context invalid: %w", err)
	}
	// 2. there is sufficient DC-money supply
	dcMoneySupply, err := c.state.GetUnit(DustCollectorMoneySupplyID, false)
	if err != nil {
		return err
	}
	if dcMoneySupply == nil {
		return fmt.Errorf("DC-money supply unit not found: id=%X", DustCollectorMoneySupplyID)
	}
	dcMoneySupplyBill, ok := dcMoneySupply.Data().(*BillData)
	if !ok {
		return errors.New("DC-money supply invalid data type")
	}

	if dcMoneySupplyBill.V < c.attr.TargetValue {
		return errors.New("insufficient DC-money supply")
	}

	// 3. tx unit id identifies an existing bill
	unitData, err := c.state.GetUnit(c.tx.UnitID(), false)
	if err != nil {
		return fmt.Errorf("target unit does not exist: %w", err)
	}
	if unitData == nil {
		return fmt.Errorf("target unit is nil id=%X", c.tx.UnitID())
	}
	if err := c.execPredicate(unitData.Bearer(), c.tx.OwnerProof, c.tx); err != nil {
		return err
	}
	billData, ok := unitData.Data().(*BillData)
	if !ok {
		return fmt.Errorf("target unit invalid data type")
	}

	// 5. bills were transferred to DC
	dustTransfers, err := c.getDCTransfers()
	if err != nil {
		return fmt.Errorf("failed to extract DC transfers: %w", err)
	}

	// 1. target value is the sum of the values of the transDC payments
	sum := c.sumDcTransferValues(dustTransfers)
	if c.attr.TargetValue != sum {
		return fmt.Errorf("target value must be equal to the sum of dust transfer values: expected %d vs provided %d", sum, c.attr.TargetValue)
	}

	if len(dustTransfers) != len(c.attr.DcTransferProofs) {
		return fmt.Errorf("invalid count of proofs: expected %d vs provided %d", len(dustTransfers), len(c.attr.DcTransferProofs))
	}
	for i, dcTx := range dustTransfers {
		// 4. transfers were in the money partition
		if dcTx.tx.TransactionOrder.SystemID() != c.systemID {
			return fmt.Errorf("dust transfer system id is not money partition system id: expected %s vs provided %s",
				c.systemID, dcTx.tx.TransactionOrder.SystemID())
		}
		// 6. transfer orders are listed in strictly increasing order of bill identifiers
		// (this ensures that no source bill can be included multiple times
		if i > 0 && bytes.Compare(dcTx.id, dustTransfers[i-1].id) != 1 {
			return errors.New("dust transfer orders are not listed in strictly increasing order of bill identifiers")
		}
		// 7. bill transfer orders contain correct target unit ids
		if !bytes.Equal(dcTx.attributes.TargetUnitID, c.tx.UnitID()) {
			return errors.New("dust transfer order target unit id is not equal to swap tx unit id")
		}
		// 8. bill transfer orders contain correct target backlinks
		if !bytes.Equal(dcTx.attributes.TargetUnitBacklink, billData.Backlink) {
			return fmt.Errorf("dust transfer target backlink is not equal to target unit backlink: expected %X vs provided %X",
				billData.Backlink, dcTx.attributes.TargetUnitBacklink)

		}
		// 9. transaction proofs of the bill transfer orders verify
		if err := types.VerifyTxProof(c.attr.DcTransferProofs[i], dcTx.tx, c.trustBase, c.hashAlgorithm); err != nil {
			return fmt.Errorf("proof is not valid: %w", err)
		}
	}
	return nil
}

func (c *swapValidationContext) isValid() error {
	if c == nil {
		return errors.New("struct is nil")
	}
	if c.tx == nil {
		return errors.New("tx is nil")
	}
	if c.attr == nil {
		return errors.New("attr is nil")
	}
	if c.state == nil {
		return errors.New("state is nil")
	}
	if c.systemID == 0 {
		return errors.New("systemID is unassigned")
	}
	if c.trustBase == nil {
		return errors.New("trust base is nil")
	}
	return nil
}

func (c *swapValidationContext) getDCTransfers() ([]*dustCollectorTransfer, error) {
	if len(c.attr.DcTransfers) == 0 {
		return nil, errors.New("tx does not contain any dust transfers")
	}
	transfers := make([]*dustCollectorTransfer, len(c.attr.DcTransfers))
	for i, t := range c.attr.DcTransfers {
		if t == nil {
			return nil, fmt.Errorf("dc tx is nil: %d", i)
		}
		a := &TransferDCAttributes{}
		if t.TransactionOrder.PayloadType() != PayloadTypeTransDC {
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

func (c *swapValidationContext) sumDcTransferValues(txs []*dustCollectorTransfer) uint64 {
	var sum uint64
	for _, dcTx := range txs {
		sum += dcTx.attributes.Value
	}
	return sum
}
