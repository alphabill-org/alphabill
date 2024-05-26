package fc

import (
	"errors"
	"fmt"

	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (f *FeeCredit) IsCredibleFC(tx *types.TransactionOrder, exeCtx txsystem.ExecutionContext) error {
	unit, err := f.state.GetUnit(tx.UnitID(), false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("state read error: %w", err)
	}
	// in case of add not found is ok and there is nothing more to be done here
	if errors.Is(err, avl.ErrNotFound) {
		if tx.PayloadType() == fcsdk.PayloadTypeAddFeeCredit {
			return nil
		}
		return fmt.Errorf("fee cerdit unit not found")
	}
	// 1. target unit is in a locked state
	if unit.IsStateLocked() {
		return fmt.Errorf("fee cerdit unit is locked")
	}
	// 2. check owner proof
	if err = f.execPredicate(unit.Bearer(), tx.OwnerProof, tx, exeCtx); err != nil {
		return fmt.Errorf("evaluating fee proof: %w", err)
	}

	// 9. the actual transaction cost does not exceed the maximum permitted by the user
	if f.feeCalculator() > tx.Payload.ClientMetadata.MaxTransactionFee {
		return errors.New("the tx fee cannot exceed the max specified fee")
	}
	return nil
}
