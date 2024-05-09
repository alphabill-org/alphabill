package evm

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	feeModule "github.com/alphabill-org/alphabill/txsystem/fc"
)

func getTransferPayloadAttributes(transfer *types.TransactionRecord) (*fc.TransferFeeCreditAttributes, error) {
	if transfer == nil {
		return nil, fmt.Errorf("transfer record is nil")
	}
	transferPayload := &fc.TransferFeeCreditAttributes{}
	if err := transfer.TransactionOrder.UnmarshalAttributes(transferPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferPayload, nil
}

func (f *FeeAccount) executeAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
	pubKey, err := predicates.ExtractPubKey(tx.OwnerProof)
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
	transferFc, err := getTransferPayloadAttributes(attr.FeeCreditTransfer)
	if err != nil {
		return nil, err
	}
	v := transferFc.Amount - attr.FeeCreditTransfer.ServerMetadata.ActualFee - fee

	// if unit exists update balance and alphabill fee credit link data
	addCredit := []state.Action{
		statedb.UpdateEthAccountAddCredit(unitID, alphaToWei(v), transferFc.LatestAdditionTime+1, tx.Hash(f.hashAlgorithm)),
		state.SetOwner(unitID, attr.FeeCreditOwnerCondition),
	}
	err = f.state.Apply(addCredit...)
	// if unable to increment credit because there unit is not found, then create one
	if err != nil && errors.Is(err, avl.ErrNotFound) {
		err = f.state.Apply(statedb.CreateAccountAndAddCredit(address, attr.FeeCreditOwnerCondition, alphaToWei(v), transferFc.LatestAdditionTime+1, tx.Hash(f.hashAlgorithm)))
	}
	if err != nil {
		return nil, fmt.Errorf("addFC state update failed: %w", err)
	}
	return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (f *FeeAccount) validateAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, exeCtx *txsystem.TxExecutionContext) error {
	// 10. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if err := feeModule.ValidateGenericFeeCreditTx(tx); err != nil {
		return fmt.Errorf("invalid fee credit transaction: %w", err)
	}
	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	pubKey, err := predicates.ExtractPubKey(tx.OwnerProof)
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
	var backlink []byte
	if u != nil {
		stateObj, ok := u.Data().(*statedb.StateObject)
		if !ok || stateObj.AlphaBill == nil {
			return fmt.Errorf("invalid fcr data")
		}
		backlink = stateObj.AlphaBill.TxHash
		// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner condition matches
		if !bytes.Equal(u.Bearer(), attr.FeeCreditOwnerCondition) {
			return fmt.Errorf("invalid owner condition: expected=%X actual=%X", u.Bearer(), attr.FeeCreditOwnerCondition)
		}
	}

	if attr.FeeCreditTransfer == nil {
		return errors.New("transferFC tx record is nil")
	}
	if attr.FeeCreditTransfer.TransactionOrder == nil {
		return errors.New("transferFC tx order is nil")
	}
	if attr.FeeCreditTransferProof == nil {
		return errors.New("transferFC tx proof is nil")
	}
	if attr.FeeCreditTransfer.ServerMetadata == nil {
		return errors.New("transferFC tx order is missing server metadata")
	}
	// 4. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	transferTx := attr.FeeCreditTransfer.TransactionOrder
	// dirty hack
	if transferTx.SystemID() != f.moneySystemID {
		return fmt.Errorf("addFC: invalid transferFC money system identifier %s (expected %s)", transferTx.SystemID(), f.moneySystemID)
	}

	if transferTx.PayloadType() != fc.PayloadTypeTransferFeeCredit {
		return fmt.Errorf("invalid transfer fee credit transation payload type: %s", transferTx.PayloadType())
	}

	transferTxAttr := &fc.TransferFeeCreditAttributes{}
	if err = transferTx.UnmarshalAttributes(transferTxAttr); err != nil {
		return fmt.Errorf("failed to unmarshal transfer fee credit attributes: %w", err)
	}

	// 5. P.A.P.A.α = P.α – bill was transferred to fee credits for this system
	if transferTxAttr.TargetSystemIdentifier != f.systemID {
		return fmt.Errorf("invalid transferFC target system identifier: expected_target_system_id: %s actual_target_system_id=%s", f.systemID, transferTxAttr.TargetSystemIdentifier)
	}

	// 6. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(transferTxAttr.TargetRecordID, tx.UnitID()) {
		return fmt.Errorf("invalid transferFC target record id: transferFC.TargetRecordId=%X tx.UnitId=%s", transferTxAttr.TargetRecordID, tx.UnitID())
	}

	// 7. (S.N[P.ι] = ⊥ ∧ P.A.P.A.η = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P.A.P.A.η = S.N[P.ι].λ) – bill transfer order contains correct target unit backlink
	if !bytes.Equal(transferTxAttr.TargetUnitBacklink, backlink) {
		return fmt.Errorf("invalid transferFC target unit backlink: transferFC.targetUnitBacklink=%X unit.backlink=%X", transferTxAttr.TargetUnitBacklink, backlink)
	}

	// 8. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	tb := transferTxAttr.EarliestAdditionTime
	te := transferTxAttr.LatestAdditionTime
	t := exeCtx.CurrentBlockNr
	if t < tb || t > te {
		return fmt.Errorf("invalid transferFC timeout: earliest=%d latest=%d current=%d", tb, te, t)
	}

	// 9. P.MC.fa + P.MC.fm ≤ P.A.P.A.v – the transaction fees can’t exceed the transferred value
	feeLimit := tx.Payload.ClientMetadata.MaxTransactionFee + attr.FeeCreditTransfer.ServerMetadata.ActualFee
	if feeLimit > transferTxAttr.Amount {
		return fmt.Errorf("invalid transferFC fee: max_fee+actual_fee=%d transferFC.Amount=%d", feeLimit, transferTxAttr.Amount)
	}

	// 3. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	if err = types.VerifyTxProof(attr.FeeCreditTransferProof, attr.FeeCreditTransfer, f.trustBase, f.hashAlgorithm); err != nil {
		return fmt.Errorf("proof is not valid: %w", err)
	}
	return nil
}
