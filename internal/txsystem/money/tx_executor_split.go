package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
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

func handleSplitTx(s *state.State, hashAlgorithm crypto.Hash, feeCalc fc.FeeCalculator) txsystem.GenericExecuteFunc[SplitAttributes] {
	return func(tx *types.TransactionOrder, attr *SplitAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing split %v", tx)
		if err := validateSplitTx(tx, attr, s); err != nil {
			return nil, fmt.Errorf("invalid split transaction: %w", err)
		}

		unitID := tx.UnitID()
		newItemID := txutil.SameShardID(unitID, HashForIDCalculation(unitID, tx.Payload.Attributes, tx.Timeout(), hashAlgorithm))

		// calculate actual tx fee cost
		fee := feeCalc()
		h := tx.Hash(hashAlgorithm)

		// update state
		if err := s.Apply(
			state.UpdateUnitData(unitID,
				func(data state.UnitData) (state.UnitData, error) {
					bd, ok := data.(*BillData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain bill data", unitID)
					}
					return &BillData{
						V:        bd.V - attr.Amount,
						T:        currentBlockNumber,
						Backlink: h,
					}, nil
				}),
			state.AddUnit(newItemID, attr.TargetBearer, &BillData{
				V:        attr.Amount,
				T:        currentBlockNumber,
				Backlink: h,
			})); err != nil {
			return nil, fmt.Errorf("unit update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID, newItemID}}, nil
	}
}

func HashForIDCalculation(idBytes []byte, attr []byte, timeout uint64, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	hasher.Write(idBytes)
	hasher.Write(attr)
	hasher.Write(util.Uint64ToBytes(timeout))
	return hasher.Sum(nil)
}

func validateSplitTx(tx *types.TransactionOrder, attr *SplitAttributes, s *state.State) error {
	data, err := s.GetUnit(tx.UnitID(), false)
	if err != nil {
		return err
	}
	return validateSplit(data.Data(), attr)
}

func validateSplit(data state.UnitData, attr *SplitAttributes) error {
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
