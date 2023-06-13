package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrInvalidBillValue       = errors.New("transaction value must be equal to bill value")
	ErrSplitBillZeroAmount    = errors.New("when splitting an bill the value assigned to the new bill must be greater than zero")
	ErrSplitBillZeroRemainder = errors.New("when splitting an bill the remaining value of the bill must be greater than zero")
	ErrInvalidDataType        = errors.New("invalid data type")
)

func handleSplitTx(state *rma.Tree, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[SplitAttributes] {
	return func(tx *types.TransactionOrder, attr *SplitAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing split %v", tx)
		if err := validateSplitTx(tx, attr, state); err != nil {
			return nil, fmt.Errorf("invalid split transaction: %w", err)
		}

		unitID := util.BytesToUint256(tx.UnitID())
		newItemId := txutil.SameShardID(unitID, HashForIDCalculation(util.Uint256ToBytes(unitID), tx.Payload.Attributes, tx.Timeout(), hashAlgorithm))

		// calculate actual tx fee cost
		fee := feeCalc()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		if err := state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.UpdateData(unitID,
				func(data rma.UnitData) (newData rma.UnitData) {
					bd, ok := data.(*BillData)
					if !ok {
						return data // todo return error
					}
					return &BillData{
						V:        bd.V - attr.Amount,
						T:        currentBlockNumber,
						Backlink: tx.Hash(hashAlgorithm),
					}
				}, h),
			rma.AddItem(newItemId, attr.TargetBearer, &BillData{
				V:        attr.Amount,
				T:        currentBlockNumber,
				Backlink: h,
			}, h)); err != nil {
			return nil, fmt.Errorf("unit update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func HashForIDCalculation(idBytes []byte, attr []byte, timeout uint64, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	hasher.Write(idBytes)
	hasher.Write(attr)
	hasher.Write(util.Uint64ToBytes(timeout))
	return hasher.Sum(nil)
}

func validateSplitTx(tx *types.TransactionOrder, attr *SplitAttributes, state *rma.Tree) error {
	data, err := state.GetUnit(util.BytesToUint256(tx.UnitID()))
	if err != nil {
		return err
	}
	return validateSplit(data.Data, attr)
}

func validateSplit(data rma.UnitData, attr *SplitAttributes) error {
	bd, ok := data.(*BillData)
	if !ok {
		return ErrInvalidDataType
	}
	if !bytes.Equal(attr.Backlink, bd.Backlink) {
		return ErrInvalidBacklink
	}

	if attr.Amount == 0 {
		return ErrSplitBillZeroAmount
	}
	if attr.RemainingValue == 0 {
		return ErrSplitBillZeroRemainder
	}

	// amount does not exceed value of the bill
	if attr.Amount >= bd.V {
		return ErrInvalidBillValue
	}
	// remaining value equals the previous value minus the amount
	if attr.RemainingValue != bd.V-attr.Amount {
		return ErrInvalidBillValue
	}
	return nil
}
