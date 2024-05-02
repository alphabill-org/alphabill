package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/state"
)

// ValidateGenericFeeCreditTx none of the free credit transactions must contain fee credit reference or separate fee authorization proof
func ValidateGenericFeeCreditTx(tx *types.TransactionOrder) error {
	// P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.GetClientFeeCreditRecordID() != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.FeeProof != nil {
		return errors.New("fee tx cannot contain fee authorization proof")
	}
	return nil
}

func VerifyMaxTxFeeDoesNotExceedFRCBalance(tx *types.TransactionOrder, fcrBalance uint64) error {
	// the transaction fees can’t exceed the fee credit record balance
	if tx.GetClientMaxTxFee() > fcrBalance {
		return fmt.Errorf("max fee cannot exceed fee credit record balance: tx.maxFee=%d fcr.Balance=%d",
			tx.GetClientMaxTxFee(), fcrBalance)
	}
	return nil
}

func ValidateCloseFC(attr *fc.CloseFeeCreditAttributes, fcr *fc.FeeCreditRecord) error {
	// verify the fee credit record is not locked
	if fcr.IsLocked() {
		return errors.New("fee credit record is locked")
	}
	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	if attr.Amount != fcr.Balance {
		return fmt.Errorf("invalid amount: amount=%d fcr.Balance=%d", attr.Amount, fcr.Balance)
	}
	if len(attr.TargetUnitID) == 0 {
		return errors.New("TargetUnitID is empty")
	}
	return nil
}

func parseFeeCreditRecord(id types.UnitID, fcrType []byte, state *state.State) (*fc.FeeCreditRecord, []byte, error) {
	if !id.HasType(fcrType) {
		return nil, nil, ErrUnitTypeIsNotFCR
	}
	bd, err := state.GetUnit(id, false)
	if err != nil {
		return nil, nil, fmt.Errorf("get fcr unit error: %w", err)
	}
	fcr, ok := bd.Data().(*fc.FeeCreditRecord)
	if !ok {
		return nil, nil, ErrUnitDataTypeIsNotFCR
	}
	return fcr, bd.Bearer(), nil
}
