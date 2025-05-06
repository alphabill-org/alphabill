package evm

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func getTransferPayloadAttributes(proof *types.TxRecordProof) (*fc.TransferFeeCreditAttributes, error) {
	if err := proof.IsValid(); err != nil {
		return nil, err
	}
	var transferPayload *fc.TransferFeeCreditAttributes
	txo, err := proof.GetTransactionOrderV1()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction order: %w", err)
	}
	if err := txo.UnmarshalAttributes(&transferPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferPayload, nil
}

func (f *FeeAccount) executeAddFC(_ *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, authProof *fc.AddFeeCreditAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	pubKey, err := predicates.ExtractPubKey(authProof.OwnerProof)
	if err != nil {
		return nil, fmt.Errorf("failed to extract public key from fee credit owner proof")
	}
	address, err := generateAddress(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to extract address from public key bytes, %w", err)
	}
	// unit id is ethereum address
	unitID := address.Bytes()
	fee := f.feeCalculator()
	// find net value of credit
	proof := attr.FeeCreditTransferProof
	transFC, err := getTransferPayloadAttributes(proof)
	if err != nil {
		return nil, err
	}
	v := transFC.Amount - proof.ActualFee() - fee

	// if unit exists update balance and alphabill fee credit link data
	addCredit := statedb.UpdateEthAccountAddCredit(unitID, alphaToWei(v), transFC.LatestAdditionTime, attr.FeeCreditOwnerPredicate)
	err = f.state.Apply(addCredit)
	// if unable to increment credit because there unit is not found, then create one
	if err != nil && errors.Is(err, avl.ErrNotFound) {
		err = f.state.Apply(statedb.CreateAccountAndAddCredit(address, attr.FeeCreditOwnerPredicate, alphaToWei(v), transFC.LatestAdditionTime))
	}
	if err != nil {
		return nil, fmt.Errorf("addFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeAccount) validateAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, authProof *fc.AddFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) error {
	// 10. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if err := feeModule.ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	pubKey, err := predicates.ExtractPubKey(authProof.OwnerProof)
	if err != nil {
		return fmt.Errorf("failed to extract public key from fee credit owner proof")
	}
	address, err := generateAddress(pubKey)
	if err != nil {
		return fmt.Errorf("failed to extract address from public key bytes, %w", err)
	}
	// unit id is ethereum address
	unitID := address.Bytes()
	u, err := f.state.GetUnit(unitID, false)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("get frc error: %w", err)
	}
	var counter *uint64
	if u != nil {
		stateObj, ok := u.Data().(*statedb.StateObject)
		abLink := stateObj.AlphaBill
		if !ok || abLink == nil {
			return fmt.Errorf("invalid fcr data")
		}
		counter = &(abLink.Counter)
		// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner predicate matches
		if !bytes.Equal(abLink.OwnerPredicate, attr.FeeCreditOwnerPredicate) {
			return fmt.Errorf("invalid owner predicate: expected=%X actual=%X", abLink.OwnerPredicate, attr.FeeCreditOwnerPredicate)
		}
	}

	feeCreditTransferProof := attr.FeeCreditTransferProof
	if err := feeCreditTransferProof.IsValid(); err != nil {
		return fmt.Errorf("transferFC transaction record proof is not valid: %w", err)
	}
	// 4. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	transferTx, err := feeCreditTransferProof.GetTransactionOrderV1()
	if err != nil {
		return fmt.Errorf("failed to get transferFC transaction order: %w", err)
	}
	// dirty hack
	if transferTx.PartitionID != f.moneyPartitionID {
		return fmt.Errorf("invalid transferFC money partition identifier %d (expected %d)", transferTx.PartitionID, f.moneyPartitionID)
	}

	if transferTx.Type != fc.TransactionTypeTransferFeeCredit {
		return fmt.Errorf("invalid transfer fee credit transation transaction type: %d", transferTx.Type)
	}

	transferTxAttr := &fc.TransferFeeCreditAttributes{}
	if err = transferTx.UnmarshalAttributes(transferTxAttr); err != nil {
		return fmt.Errorf("failed to unmarshal transfer fee credit attributes: %w", err)
	}

	// 5. P.A.P.A.α = P.α – bill was transferred to fee credits for this system
	if transferTxAttr.TargetPartitionID != f.partitionID {
		return fmt.Errorf("invalid transferFC target partition identifier: expected_target_partition_id: %s actual_target_partition_id=%s", f.partitionID, transferTxAttr.TargetPartitionID)
	}

	// 6. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(transferTxAttr.TargetRecordID, tx.UnitID) {
		return fmt.Errorf("invalid transferFC target record id: transferFC.TargetRecordId=%X tx.UnitId=%s", transferTxAttr.TargetRecordID, tx.UnitID)
	}

	// 7. (S.N[P.ι] = ⊥ ∧ P.A.P.A.η = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P.A.P.A.η = S.N[P.ι].λ) – bill transfer order contains correct target unit counter
	if !intPtrsEqual(transferTxAttr.TargetUnitCounter, counter) {
		return fmt.Errorf("invalid transferFC target unit counter: transferFC.targetUnitCounter=%v unit.counter=%v", fromPtrOr(transferTxAttr.TargetUnitCounter, "<nil>"), fromPtrOr(counter, "<nil>"))
	}

	// 8. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	if exeCtx.CurrentRound() > transferTxAttr.LatestAdditionTime {
		return fmt.Errorf("invalid transferFC timeout: latestAdditionTime=%d currentRoundNumber=%d", transferTxAttr.LatestAdditionTime, exeCtx.CurrentRound())
	}

	// 9. P.MC.fa + P.MC.fm ≤ P.A.P.A.v – the transaction fees can’t exceed the transferred value
	feeLimit := tx.MaxFee() + feeCreditTransferProof.ActualFee()
	if feeLimit > transferTxAttr.Amount {
		return fmt.Errorf("invalid transferFC fee: max_fee+actual_fee=%d transferFC.Amount=%d", feeLimit, transferTxAttr.Amount)
	}

	// 3. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	if err = feeCreditTransferProof.Verify(f.orchestration.TrustBase); err != nil {
		return fmt.Errorf("proof is not valid: %w", err)
	}
	return nil
}

func fromPtrOr(x *uint64, fallback string) string {
	if x == nil {
		return fallback
	}
	return strconv.FormatUint(*x, 10)
}

func intPtrsEqual(x, y *uint64) bool {
	if x == nil && y == nil {
		return true
	}
	if x == nil || y == nil {
		return false
	}
	return *x == *y
}
