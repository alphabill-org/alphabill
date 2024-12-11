package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeAccount) executeCloseFC(tx *types.TransactionOrder, attr *fc.CloseFeeCreditAttributes, authProof *fc.CloseFeeCreditAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	pubKey, err := predicates.ExtractPubKey(authProof.OwnerProof)
	if err != nil {
		return nil, fmt.Errorf("failed to extract public key from fee credit owner proof")
	}
	addr, err := generateAddress(pubKey)
	if err != nil {
		return nil, fmt.Errorf("execution failed to extract address from public key bytes: %w", err)
	}
	// decrement credit and update AB FCR counter
	if err := f.state.Apply(statedb.UpdateEthAccountCloseCredit(unitID, alphaToWei(attr.Amount))); err != nil {
		return nil, fmt.Errorf("closeFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: f.feeCalculator(), TargetUnits: []types.UnitID{addr.Bytes()}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeAccount) validateCloseFC(tx *types.TransactionOrder, attr *fc.CloseFeeCreditAttributes, authProof *fc.CloseFeeCreditAuthProof, _ txtypes.ExecutionContext) error {
	// there’s no fee credit reference or separate fee authorization proof
	if err := feeModule.ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	// ι identifies an existing fee credit record
	// ExtrType(P.ι) = fcr – target unit is a fee credit record
	// S.N[P.ι] != ⊥ - ι identifies an existing fee credit record
	pubKey, err := predicates.ExtractPubKey(authProof.OwnerProof)
	if err != nil {
		return fmt.Errorf("failed to extract public key from fee credit owner proof")
	}
	addr, err := generateAddress(pubKey)
	if err != nil {
		return fmt.Errorf("failed to extract address from public key bytes: %w", err)
	}
	unitID := addr.Bytes()
	u, err := f.state.GetUnit(unitID, false)
	if u == nil || err != nil {
		return fmt.Errorf("get fcr unit error: %w", err)
	}
	stateObj, ok := u.Data().(*statedb.StateObject)
	if !ok {
		return fmt.Errorf("invalid unit type: not evm object")
	}
	fcr := &fc.FeeCreditRecord{
		Balance:     weiToAlpha(stateObj.Account.Balance),
		Counter:     stateObj.AlphaBill.Counter,
		MinLifetime: stateObj.AlphaBill.MinLifetime,
	}
	// verify the fee credit record is not locked
	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	// target unit list is empty
	if err = feeModule.ValidateCloseFC(attr, fcr); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}
	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if err = feeModule.VerifyMaxTxFeeDoesNotExceedFRCBalance(tx, fcr.Balance); err != nil {
		return fmt.Errorf("not enough funds: %w", err)
	}
	return nil
}
