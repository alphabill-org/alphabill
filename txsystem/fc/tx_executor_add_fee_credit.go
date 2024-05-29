package fc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
)

func (f *FeeCredit) executeAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, exeCtx txsystem.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.UnitID()
	// calculate actual tx fee cost
	fee := f.feeCalculator()
	// find net value of credit
	transferFc, err := getTransferPayloadAttributes(attr.FeeCreditTransfer)
	if err != nil {
		return nil, err
	}

	v := transferFc.Amount - attr.FeeCreditTransfer.ServerMetadata.ActualFee - fee

	err = f.state.Apply(unit.IncrCredit(unitID, v, transferFc.LatestAdditionTime))
	// if unable to increment credit because there unit is not found, then create one
	if err != nil && errors.Is(err, avl.ErrNotFound) {
		// add credit
		fcr := &fc.FeeCreditRecord{
			Balance: v,
			Counter: 0,
			Timeout: transferFc.LatestAdditionTime,
			Locked:  0,
		}
		err = f.state.Apply(unit.AddCredit(unitID, attr.FeeCreditOwnerCondition, fcr))
	}
	if err != nil {
		return nil, fmt.Errorf("addFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeCredit) validateAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, exeCtx txsystem.ExecutionContext) error {
	// 12. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if err := ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	transTxr := attr.FeeCreditTransfer
	if transTxr == nil {
		return errors.New("transferFC tx record is nil")
	}
	transTxo := transTxr.TransactionOrder
	if transTxo == nil {
		return errors.New("transferFC tx order is nil")
	}
	if attr.FeeCreditTransferProof == nil {
		return errors.New("transferFC tx proof is nil")
	}
	if transTxr.ServerMetadata == nil {
		return errors.New("transferFC tx order is missing server metadata")
	}
	if transTxo.PayloadType() != fc.PayloadTypeTransferFeeCredit {
		return fmt.Errorf("invalid transfer fee credit transaction payload type: %s", transTxo.PayloadType())
	}
	transAttr, err := getTransferPayloadAttributes(transTxr)
	if err != nil {
		return fmt.Errorf("transfer tx attributes error: %w", err)
	}

	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	fcr, bearer, err := parseFeeCreditRecord(tx.UnitID(), f.feeCreditRecordUnitType, f.state)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("get fcr error: %w", err)
	}
	if bearer != nil {
		// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner condition matches
		if !bytes.Equal(bearer, attr.FeeCreditOwnerCondition) {
			return fmt.Errorf("invalid owner condition: expected=%X actual=%X", bearer, attr.FeeCreditOwnerCondition)
		}
		// "framework level" owner check:
		// 4. S.N[P.ι] = ⊥ ∨ VerifyOwner(S.N[P.ι].φ, P, P.s) = 1 – if the target exists the owner proof must verify
		if err = f.execPredicate(bearer, tx.OwnerProof, tx, exeCtx); err != nil {
			return fmt.Errorf("executing fee credit predicate on existing unit: %w", err)
		}
	} else {
		// 3. S.N[P.ι] != ⊥ ∨ ExtrUnit(P.ι) = PrndSh(ExtrUnit(P.ι), P.A.φ|P′.A.t′) – if the target does not exist, the identifier must agree with the owner predicate
		fcrID := f.NewFeeCreditRecordID(tx.UnitID(), attr.FeeCreditOwnerCondition, transAttr.LatestAdditionTime)
		if !fcrID.Eq(tx.UnitID()) {
			return fmt.Errorf("tx.unitID is not equal to expected fee credit record id (hash of owner predicate), tx.UnitID=%s expected.fcrID=%s", tx.UnitID(), fcrID)
		}
		// 4. S.N[P.ι] != ⊥ ∨ VerifyOwner(P.A.φ, P, P.s) = 1 – if the target does not exist, the owner proof must verify
		if err = f.execPredicate(attr.FeeCreditOwnerCondition, tx.OwnerProof, tx, exeCtx); err != nil {
			return fmt.Errorf("executing fee credit predicate: %w", err)
		}
	}
	// 6. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	if transTxo.SystemID() != f.moneySystemIdentifier {
		return fmt.Errorf("addFC: invalid transferFC money system identifier %s (expected %s)", transTxo.SystemID(), f.moneySystemIdentifier)
	}

	// 7. P.A.P.A.α = P.α – bill was transferred to fee credits for this system
	if transAttr.TargetSystemIdentifier != f.systemIdentifier {
		return fmt.Errorf("invalid transferFC target system identifier: expected_target_system_id: %s actual_target_system_id=%s", f.systemIdentifier, transAttr.TargetSystemIdentifier)
	}

	// 8. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(transAttr.TargetRecordID, tx.UnitID()) {
		return fmt.Errorf("invalid transferFC target record id: transferFC.TargetRecordId=%s tx.UnitId=%s", types.UnitID(transAttr.TargetRecordID), tx.UnitID())
	}

	// 9. (S.N[P.ι] = ⊥ ∧ P′.A.c′ = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P′.A.c′ = S.N[P.ι].c) – bill transfer order contains correct target unit counter value
	if fcr == nil {
		if transAttr.TargetUnitCounter != nil {
			return errors.New("invalid transferFC target unit counter (target counter must be nil if creating fee credit record for the first time)")
		}
	} else {
		if transAttr.TargetUnitCounter == nil {
			return errors.New("invalid transferFC target unit counter (target counter must not be nil if updating existing fee credit record)")
		}
		if fcr.GetCounter() != *transAttr.TargetUnitCounter {
			return fmt.Errorf("invalid transferFC target unit counter: transferFC.targetUnitCounter=%d unit.counter=%d", *transAttr.TargetUnitCounter, fcr.GetCounter())
		}
	}

	// 10. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	if exeCtx.CurrentRound() > transAttr.LatestAdditionTime {
		return fmt.Errorf("invalid transferFC timeout: latestAdditionTime=%d currentRoundNumber=%d", transAttr.LatestAdditionTime, exeCtx.CurrentRound())
	}

	// 11. P.MC.fa + P.MC.fm ≤ P.A.P.A.v – the transaction fees can’t exceed the transferred value
	feeLimit := tx.Payload.ClientMetadata.MaxTransactionFee + transTxr.ServerMetadata.ActualFee
	if feeLimit > transAttr.Amount {
		return fmt.Errorf("invalid transferFC fee: max_fee+actual_fee=%d transferFC.Amount=%d", feeLimit, transAttr.Amount)
	}

	// 5. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	if err = types.VerifyTxProof(attr.FeeCreditTransferProof, transTxr, f.trustBase, f.hashAlgorithm); err != nil {
		return fmt.Errorf("proof is not valid: %w", err)
	}
	return nil
}

func getTransferPayloadAttributes(transfer *types.TransactionRecord) (*fc.TransferFeeCreditAttributes, error) {
	transferPayload := &fc.TransferFeeCreditAttributes{}
	if err := transfer.TransactionOrder.UnmarshalAttributes(transferPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferPayload, nil
}

func (f *FeeCredit) NewFeeCreditRecordID(unitID []byte, ownerPredicate []byte, timeout uint64) types.UnitID {
	unitPart := fc.NewFeeCreditRecordUnitPart(ownerPredicate, timeout)
	unitIdLen := len(unitPart) + len(f.feeCreditRecordUnitType)
	return types.NewUnitID(unitIdLen, unitID, unitPart, f.feeCreditRecordUnitType)
}
