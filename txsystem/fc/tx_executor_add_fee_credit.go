package fc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeCreditModule) executeAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, _ *fc.AddFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	fee := exeCtx.CalculateCost()

	// find net value of credit
	transferProof := attr.FeeCreditTransferProof
	transferAttributes, err := getTransferFC(transferProof)
	if err != nil {
		return nil, err
	}
	newBalance := transferAttributes.Amount - transferProof.ActualFee() - fee

	err = f.state.Apply(unit.IncrCredit(unitID, newBalance, transferAttributes.LatestAdditionTime))
	// if unable to increment credit because there unit is not found, then create one
	if err != nil && errors.Is(err, avl.ErrNotFound) {
		// add credit
		fcr := &fc.FeeCreditRecord{
			Balance: newBalance,
			Counter: 0,
			Timeout: transferAttributes.LatestAdditionTime,
			Locked:  0,
		}
		err = f.state.Apply(unit.AddCredit(unitID, attr.FeeCreditOwnerPredicate, fcr))
	}
	if err != nil {
		return nil, fmt.Errorf("addFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeCreditModule) validateAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, authProof *fc.AddFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	if err := ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("fee credit transaction validation error: %w", err)
	}

	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	fcr, fcrOwnerPredicate, err := parseFeeCreditRecord(tx.UnitID, f.feeCreditRecordUnitType, f.state)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("get fcr error: %w", err)
	}
	createFC := errors.Is(err, avl.ErrNotFound)
	transAttr, err := f.checkTransferFC(tx, attr, exeCtx)
	if err != nil {
		return fmt.Errorf("add fee credit validation failed: %w", err)
	}
	// either create fee credit or add to existing
	if createFC {
		// try to create free credit
		// 3. S.N[P.ι] != ⊥ ∨ ExtrUnit(P.ι) = PrndSh(ExtrUnit(P.ι), P.A.φ|P′.A.t′) – if the target does not exist, the identifier must agree with the owner predicate
		fcrID := f.NewFeeCreditRecordID(tx.UnitID, attr.FeeCreditOwnerPredicate, transAttr.LatestAdditionTime)
		if !fcrID.Eq(tx.UnitID) {
			return fmt.Errorf("tx.unitID is not equal to expected fee credit record id (hash of owner predicate), tx.UnitID=%s expected.fcrID=%s", tx.UnitID, fcrID)
		}
		// on first create the transfer counter must not be present
		if transAttr.TargetUnitCounter != nil {
			return errors.New("invalid transferFC target unit counter (target counter must be nil if creating fee credit record for the first time)")
		}
		// 4. S.N[P.ι] != ⊥ ∨ VerifyOwner(P.A.φ, P, P.s) = 1 – if the target does not exist, the owner proof must verify
		if err = f.execPredicate(attr.FeeCreditOwnerPredicate, authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
			return fmt.Errorf("executing fee credit predicate: %w", err)
		}
	} else {
		// add to existing fee credit
		// 9. (S.N[P.ι] != ⊥ ∧ P′.A.c′ = S.N[P.ι].c) – bill transfer order contains correct target unit counter value
		if transAttr.TargetUnitCounter == nil {
			return errors.New("invalid transferFC target unit counter (target counter must not be nil if updating existing fee credit record)")
		}
		// FCR counter must match transfer counter
		if fcr.GetCounter() != *transAttr.TargetUnitCounter {
			return fmt.Errorf("invalid transferFC target unit counter: transferFC.targetUnitCounter=%d unit.counter=%d", *transAttr.TargetUnitCounter, fcr.GetCounter())
		}
		// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner predicate matches
		if !bytes.Equal(fcrOwnerPredicate, attr.FeeCreditOwnerPredicate) {
			return fmt.Errorf("invalid owner predicate: expected=%X actual=%X", fcrOwnerPredicate, attr.FeeCreditOwnerPredicate)
		}
	}
	// 5. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	if err = types.VerifyTxProof(attr.FeeCreditTransferProof, f.trustBase, f.hashAlgorithm); err != nil {
		return fmt.Errorf("proof is not valid: %w", err)
	}
	transferCreditCost := attr.FeeCreditTransferProof.ActualFee()
	addCreditCost := transferCreditCost + exeCtx.CalculateCost()
	if transAttr.Amount < transferCreditCost+addCreditCost {
		return fmt.Errorf("add fee credit costs more %v than amount transfered %v", addCreditCost, transAttr.Amount)
	}
	return nil
}

func (f *FeeCreditModule) checkTransferFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, exeCtx txtypes.ExecutionContext) (*fc.TransferFeeCreditAttributes, error) {
	transProof := attr.FeeCreditTransferProof
	if err := transProof.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid transferFC transaction record proof: %w", err)
	}
	transAttr, err := getTransferFC(transProof)
	if err != nil {
		return nil, fmt.Errorf("transfer transaction attributes error: %w", err)
	}
	// 6. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	systemID := transProof.TransactionOrder().SystemID
	if systemID != f.moneySystemIdentifier {
		return nil, fmt.Errorf("invalid transferFC money system identifier %s (expected %s)", systemID, f.moneySystemIdentifier)
	}
	// 7. P.A.P.A.α = P.α – bill was transferred to fee credits for this system
	if transAttr.TargetSystemIdentifier != f.systemIdentifier {
		return nil, fmt.Errorf("invalid transferFC target system identifier: expected_target_system_id: %s actual_target_system_id=%s", f.systemIdentifier, transAttr.TargetSystemIdentifier)
	}
	// 8. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(transAttr.TargetRecordID, tx.UnitID) {
		return nil, fmt.Errorf("invalid transferFC target record id: transferFC.TargetRecordId=%s tx.UnitId=%s", types.UnitID(transAttr.TargetRecordID), tx.UnitID)
	}

	// 10. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	if exeCtx.CurrentRound() > transAttr.LatestAdditionTime {
		return nil, fmt.Errorf("invalid transferFC timeout: latestAdditionTime=%d currentRoundNumber=%d", transAttr.LatestAdditionTime, exeCtx.CurrentRound())
	}
	// 11. P.MC.fa + P.MC.fm ≤ P.A.P.A.v – the transaction fees can’t exceed the transferred value
	feeLimit := tx.MaxFee() + transProof.ActualFee()
	if feeLimit > transAttr.Amount {
		return nil, fmt.Errorf("invalid transferFC fee: max_fee+actual_fee=%d transferFC.Amount=%d", feeLimit, transAttr.Amount)
	}
	return transAttr, nil
}

func getTransferFC(addFeeCreditProof *types.TxRecordProof) (*fc.TransferFeeCreditAttributes, error) {
	txo := addFeeCreditProof.TransactionOrder()
	txType := txo.Type
	if txType != fc.TransactionTypeTransferFeeCredit {
		return nil, fmt.Errorf("invalid transfer fee credit transaction transaction type: %d", txType)
	}
	transferAttributes := &fc.TransferFeeCreditAttributes{}
	if err := txo.UnmarshalAttributes(transferAttributes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferAttributes, nil
}

func (f *FeeCreditModule) NewFeeCreditRecordID(unitID []byte, ownerPredicate []byte, timeout uint64) types.UnitID {
	unitPart := fc.NewFeeCreditRecordUnitPart(ownerPredicate, timeout)
	unitIdLen := len(unitPart) + len(f.feeCreditRecordUnitType)
	return types.NewUnitID(unitIdLen, unitID, unitPart, f.feeCreditRecordUnitType)
}
