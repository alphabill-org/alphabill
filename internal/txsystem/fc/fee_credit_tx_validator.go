package fc

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
)

type (
	// DefaultFeeCreditTxValidator default validator for partition specific
	// "add fee credit" and "close fee credit" transactions
	DefaultFeeCreditTxValidator struct {
		moneySystemID []byte
		systemID      []byte
		hashAlgorithm crypto.Hash
		verifiers     map[string]abcrypto.Verifier
	}

	AddFCValidationContext struct {
		Tx                 *transactions.AddFeeCreditWrapper
		Unit               *rma.Unit
		CurrentRoundNumber uint64
	}

	CloseFCValidationContext struct {
		Tx   *transactions.CloseFeeCreditWrapper
		Unit *rma.Unit
	}
)

func NewDefaultFeeCreditTxValidator(moneySystemID, systemID []byte, hashAlgorithm crypto.Hash, verifiers map[string]abcrypto.Verifier) *DefaultFeeCreditTxValidator {
	return &DefaultFeeCreditTxValidator{
		moneySystemID: moneySystemID,
		systemID:      systemID,
		hashAlgorithm: hashAlgorithm,
		verifiers:     verifiers,
	}
}

func (v *DefaultFeeCreditTxValidator) ValidateAddFeeCredit(ctx *AddFCValidationContext) error {
	if ctx == nil {
		return errors.New("validation context is nil")
	}
	tx := ctx.Tx
	if tx == nil {
		return errors.New("tx is nil")
	}

	// 10. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.Transaction.ClientMetadata.FeeCreditRecordId != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.Transaction.FeeProof != nil {
		return errors.New("fee tx cannot contain fee authorization proof")
	}

	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	// TODO

	// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner condition matches
	unit := ctx.Unit
	if unit != nil && !bytes.Equal(unit.Bearer, tx.AddFC.FeeCreditOwnerCondition) {
		return fmt.Errorf("invalid owner condition: expected=%X actual=%X", unit.Bearer, tx.AddFC.FeeCreditOwnerCondition)
	}

	// 4. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	if !bytes.Equal(tx.AddFC.FeeCreditTransfer.SystemId, v.moneySystemID) {
		return fmt.Errorf("invalid transferFC system identifier: expected_system_id=%X actual_system_id=%X", v.moneySystemID, tx.AddFC.FeeCreditTransfer.SystemId)
	}

	// 5. P.A.P.A.α = P.α – bill was transferred to fee credits for this system
	if !bytes.Equal(tx.TransferFC.TransferFC.TargetSystemIdentifier, v.systemID) {
		return fmt.Errorf("invalid transferFC target system identifier: expected_target_system_id: %X actual_target_system_id=%X", v.systemID, tx.TransferFC.TransferFC.TargetSystemIdentifier)
	}

	// 6. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(tx.TransferFC.TransferFC.TargetRecordId, tx.Transaction.UnitId) {
		return fmt.Errorf("invalid transferFC target record id: transferFC.TargetRecordId=%X tx.UnitId=%X", tx.TransferFC.TransferFC.TargetRecordId, tx.Transaction.UnitId)
	}

	// 7. (S.N[P.ι] = ⊥ ∧ P.A.P.A.η = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P.A.P.A.η = S.N[P.ι].λ) – bill transfer order contains correct nonce
	var backlink []byte
	if unit != nil {
		backlink = unit.StateHash
	}
	if !bytes.Equal(tx.TransferFC.TransferFC.Nonce, backlink) {
		return fmt.Errorf("invalid transferFC nonce: transferFC.nonce=%X unit.backlink=%X", tx.TransferFC.TransferFC.Nonce, backlink)
	}

	// 8. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	tb := tx.TransferFC.TransferFC.EarliestAdditionTime
	te := tx.TransferFC.TransferFC.LatestAdditionTime
	t := ctx.CurrentRoundNumber
	if t < tb || t > te {
		return fmt.Errorf("invalid transferFC timeout: earliest=%d latest=%d current=%d", tb, te, t)
	}

	// 9. P.MC.fm ≤ P.A.P.A.v – the transaction fee can’t exceed the transferred value
	if tx.Transaction.ClientMetadata.MaxFee > tx.TransferFC.TransferFC.Amount {
		return fmt.Errorf("invalid transferFC fee: max_fee=%d transferFC.Amount=%d", tx.Transaction.ClientMetadata.MaxFee, tx.TransferFC.TransferFC.Amount)
	}

	// 3. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	proof := tx.AddFC.FeeCreditTransferProof
	if proof.ProofType != block.ProofType_PRIM {
		return fmt.Errorf("invalid proof type: expected=%s actual=%s", block.ProofType_PRIM, proof.ProofType)
	}
	err := proof.Verify(tx.AddFC.FeeCreditTransfer.UnitId, tx.TransferFC, v.verifiers, v.hashAlgorithm)
	if err != nil {
		return fmt.Errorf("proof is not valid: %w", err)
	}
	return nil
}

func (v *DefaultFeeCreditTxValidator) ValidateCloseFC(ctx *CloseFCValidationContext) error {
	if ctx == nil {
		return errors.New("validation context is nil")
	}
	tx := ctx.Tx
	if tx == nil {
		return errors.New("tx is nil")
	}

	// P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.Transaction.ClientMetadata.FeeCreditRecordId != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.Transaction.FeeProof != nil {
		return errors.New("fee tx cannot contain fee authorization proof")
	}

	// S.N[P.ι] != ⊥ - ι identifies an existing fee credit record
	if ctx.Unit == nil {
		return errors.New("unit is nil")
	}
	fcr, ok := ctx.Unit.Data.(*FeeCreditRecord)
	if !ok {
		return errors.New("unit data is not of type fee credit record")
	}

	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	if tx.CloseFC.Amount != fcr.Balance {
		return fmt.Errorf("invalid amount: amount=%d fcr.Balance=%d", tx.CloseFC.Amount, fcr.Balance)
	}

	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if tx.Transaction.ClientMetadata.MaxFee > fcr.Balance {
		return fmt.Errorf("invalid fee: max_fee=%d fcr.Balance=%d", tx.Transaction.ClientMetadata.MaxFee, fcr.Balance)
	}
	return nil
}
