package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrUnitTypeIsNotFCR     = errors.New("invalid unit identifier: type is not fee credit record")
	ErrUnitDataTypeIsNotFCR = errors.New("invalid unit type: unit is not fee credit record")
)

// ValidateGenericFeeCreditTx none of the fee credit transactions must contain fee credit reference or separate fee authorization proof
func ValidateGenericFeeCreditTx(tx *types.TransactionOrder) error {
	// P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.FeeCreditRecordID() != nil {
		return errors.New("fee transaction cannot contain fee credit reference")
	}
	if tx.FeeProof != nil {
		return errors.New("fee transaction cannot contain fee authorization proof")
	}
	return nil
}

func VerifyMaxTxFeeDoesNotExceedFRCBalance(tx *types.TransactionOrder, fcrBalance uint64) error {
	// the transaction fees can’t exceed the fee credit record balance
	if tx.MaxFee() > fcrBalance {
		return fmt.Errorf("max fee cannot exceed fee credit record balance: tx.maxFee=%d fcr.Balance=%d",
			tx.MaxFee(), fcrBalance)
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
	if attr.Counter != fcr.Counter {
		return fmt.Errorf("invalid counter: counter=%d fcr.Counter=%d", attr.Counter, fcr.Counter)
	}
	if len(attr.TargetUnitID) == 0 {
		return errors.New("TargetUnitID is empty")
	}
	return nil
}

func parseFeeCreditRecord(id types.UnitID, fcrType []byte, state *state.State) (*fc.FeeCreditRecord, error) {
	bd, err := state.GetUnit(id, false)
	if err != nil {
		return nil, fmt.Errorf("get fcr unit error: %w", err)
	}
	if !id.HasType(fcrType) {
		return nil, ErrUnitTypeIsNotFCR
	}
	fcr, ok := bd.Data().(*fc.FeeCreditRecord)
	if !ok {
		return nil, ErrUnitDataTypeIsNotFCR
	}
	return fcr, nil
}
