package fc

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

type (
	FeeBalanceValidator struct {
		state                   StateReader
		execPredicate           predicates.PredicateRunner
		feeCreditRecordUnitType []byte
	}

	StateReader interface {
		GetUnit(id types.UnitID, committed bool) (*state.Unit, error)
	}
)

func NewFeeBalanceValidator(stateReader StateReader, execPredicate predicates.PredicateRunner, feeCreditRecordUnitType []byte) *FeeBalanceValidator {
	return &FeeBalanceValidator{
		state:                   stateReader,
		execPredicate:           execPredicate,
		feeCreditRecordUnitType: feeCreditRecordUnitType,
	}
}

/*
IsCredible implements the fee credit verification for ordinary transactions (everything else except fee credit txs)
*/
func (f *FeeBalanceValidator) IsCredible(exeCtx txtypes.ExecutionContext, tx *types.TransactionOrder) error {
	// 1. ExtrType(ιf) = fcr ∧ N[ιf] != ⊥ – the fee payer has credit in this system
	fcrID := tx.FeeCreditRecordID()
	if len(fcrID) == 0 {
		return errors.New("fee credit record missing")
	}
	if !types.UnitID(fcrID).HasType(f.feeCreditRecordUnitType) {
		return errors.New("invalid fee credit record id type")
	}
	fcrUnit, _ := f.state.GetUnit(fcrID, false)
	if fcrUnit == nil {
		return errors.New("fee credit record unit is nil")
	}
	fcr, ok := fcrUnit.Data().(*fc.FeeCreditRecord)
	if !ok {
		return errors.New("invalid fee credit record type")
	}
	// 2. the maximum permitted transaction cost does not exceed the fee credit balance
	if fcr.Balance < tx.MaxFee() {
		return fmt.Errorf("the max fee cannot exceed fee credit balance. FC balance %d vs max fee %d", fcr.Balance, tx.MaxFee())
	}
	// VerifyFeeAuth(N[ιf].φ, T, T.sf) - fee authorization proof satisfies the owner predicate of the fee credit record
	if err := f.execPredicate(fcr.OwnerPredicate, tx.FeeProof, tx.FeeProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("evaluating fee proof: %w", err)
	}
	return nil
}
