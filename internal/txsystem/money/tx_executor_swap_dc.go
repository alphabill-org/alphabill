package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/holiman/uint256"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	dustCollectorTransfer struct {
		id         *uint256.Int
		tx         *types.TransactionRecord
		attributes *TransferDCAttributes
	}
	swapValidationContext struct {
		tx            *types.TransactionOrder
		attr          *SwapDCAttributes
		state         stateProvider
		systemID      []byte
		hashAlgorithm crypto.Hash
		trustBase     map[string]abcrypto.Verifier
	}
	stateProvider interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
	}
)

func handleSwapDCTx(s *state.State, systemID []byte, hashAlgorithm crypto.Hash, trustBase map[string]abcrypto.Verifier, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[SwapDCAttributes] {
	return func(tx *types.TransactionOrder, attr *SwapDCAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing swap %v", tx)
		c := &swapValidationContext{
			tx:            tx,
			attr:          attr,
			state:         s,
			systemID:      systemID,
			hashAlgorithm: hashAlgorithm,
			trustBase:     trustBase,
		}
		if err := c.validateSwapTx(); err != nil {
			return nil, fmt.Errorf("invalid swap transaction: %w", err)
		}
		// calculate actual tx fee cost
		fee := feeCalc()

		h := tx.Hash(hashAlgorithm)

		// reduce dc-money supply by target value and update timeout and backlink
		updateDCMoneySupplyFn := state.UpdateUnitData(dustCollectorMoneySupplyID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", dustCollectorMoneySupplyID)
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
				return bd, nil
			})
		if err := s.Apply(updateDCMoneySupplyFn, updateTargetUnitFn); err != nil {
			return nil, fmt.Errorf("unit update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func (c *swapValidationContext) validateSwapTx() error {
	if err := c.isValid(); err != nil {
		return fmt.Errorf("swap validation context invalid: %w", err)
	}
	// 2. there is sufficient DC-money supply
	dcMoneySupply, err := c.state.GetUnit(dustCollectorMoneySupplyID, false)
	if err != nil {
		return err
	}
	if dcMoneySupply == nil {
		return fmt.Errorf("DC-money supply unit not found: id=%X", dustCollectorMoneySupplyID)
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
		if !bytes.Equal(dcTx.tx.TransactionOrder.SystemID(), c.systemID) {
			return fmt.Errorf("dust transfer system id is not money partition system id: expected %X vs provided %X",
				c.systemID, dcTx.tx.TransactionOrder.SystemID())
		}
		// 6. transfer orders are listed in strictly increasing order of bill identifiers
		// (this ensures that no source bill can be included multiple times
		if i > 0 && !dcTx.id.Gt(dustTransfers[i-1].id) {
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
	if c.systemID == nil {
		return errors.New("systemID is nil")
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
			id:         util.BytesToUint256(t.TransactionOrder.UnitID()),
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
