package validator

import (
	"bytes"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

var (
	ErrValidationContextNil = errors.New("validation context is nil")
	ErrTxIsNil              = errors.New("tx is nil")
	ErrRecordIDExists       = errors.New("fee tx cannot contain fee credit reference")
	ErrFeeProofExists       = errors.New("fee tx cannot contain fee authorization proof")
	ErrInvalidProofType     = errors.New("invalid proof type")

	// add fee credit errors
	ErrAddFCInvalidCloseFCType    = errors.New("addFC: invalid nested transferFC tx type")
	ErrAddFCInvalidOwnerCondition = errors.New("addFC: invalid owner condition")
	ErrAddFCInvalidSystemID       = errors.New("addFC: invalid transferFC system identifier")
	ErrAddFCInvalidTargetSystemID = errors.New("addFC: invalid transferFC target system identifier")
	ErrAddFCInvalidTargetRecordID = errors.New("addFC: invalid transferFC target record id")
	ErrAddFCInvalidNonce          = errors.New("addFC: invalid transferFC nonce")
	ErrAddFCInvalidTimeout        = errors.New("addFC: invalid transferFC timeout")
	ErrAddFCInvalidTxFee          = errors.New("addFC: invalid transferFC fee")

	// close fee credit errors
	ErrCloseFCUnitIsNil       = errors.New("closeFC: unit is nil")
	ErrCloseFCInvalidUnitType = errors.New("closeFC: unit data is not of type fee credit record")
	ErrCloseFCInvalidAmount   = errors.New("closeFC: invalid amount")
	ErrCloseFCInvalidFee      = errors.New("closeFC: invalid fee")
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
		Tx                 *fc.AddFeeCreditWrapper
		Unit               *rma.Unit
		CurrentRoundNumber uint64
	}

	CloseFCValidationContext struct {
		Tx   *fc.CloseFeeCreditWrapper
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

func (v *DefaultFeeCreditTxValidator) ValidateAddFC(ctx *AddFCValidationContext) error {
	if ctx == nil {
		return ErrValidationContextNil
	}
	tx := ctx.Tx
	if tx == nil {
		return ErrTxIsNil
	}

	// 10. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.Transaction.ClientMetadata.FeeCreditRecordId != nil {
		return ErrRecordIDExists
	}
	if tx.Transaction.FeeProof != nil {
		return ErrFeeProofExists
	}

	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	// TODO

	// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner condition matches
	unit := ctx.Unit
	if unit != nil {
		if !bytes.Equal(unit.Bearer, tx.AddFC.FeeCreditOwnerCondition) {
			return ErrAddFCInvalidOwnerCondition
		}
	}

	// wrap nested transferFC to domain
	transferFC, err := fc.NewFeeCreditTx(tx.AddFC.FeeCreditTransfer)
	if err != nil {
		return err
	}
	transferFCWrapper, ok := transferFC.(*fc.TransferFeeCreditWrapper)
	if !ok {
		return ErrAddFCInvalidCloseFCType
	}

	// 4. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	if !bytes.Equal(tx.AddFC.FeeCreditTransfer.SystemId, v.moneySystemID) {
		return ErrAddFCInvalidSystemID
	}

	// 5. P.A.P.A.α = P.α – bill was transferred to fee credits for this system
	if !bytes.Equal(transferFCWrapper.TransferFC.TargetSystemIdentifier, v.systemID) {
		return ErrAddFCInvalidTargetSystemID
	}

	// 6. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(transferFCWrapper.TransferFC.TargetRecordId, tx.Transaction.UnitId) {
		return ErrAddFCInvalidTargetRecordID
	}

	// 7. (S.N[P.ι] = ⊥ ∧ P.A.P.A.η = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P.A.P.A.η = S.N[P.ι].λ) – bill transfer order contains correct nonce
	if unit == nil {
		if len(transferFCWrapper.TransferFC.Nonce) > 0 {
			return ErrAddFCInvalidNonce
		}
	} else {
		if !bytes.Equal(transferFCWrapper.TransferFC.Nonce, unit.StateHash) {
			return ErrAddFCInvalidNonce
		}
	}

	// 8. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	tb := transferFCWrapper.TransferFC.EarliestAdditionTime
	te := transferFCWrapper.TransferFC.LatestAdditionTime
	t := ctx.CurrentRoundNumber
	if t < tb || t > te {
		return ErrAddFCInvalidTimeout
	}

	// 9. P.MC.fm ≤ P.A.P.A.v – the transaction fee can’t exceed the transferred value
	if tx.Transaction.ClientMetadata.MaxFee > transferFCWrapper.TransferFC.Amount {
		return ErrAddFCInvalidTxFee
	}

	// 3. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	proof := tx.AddFC.FeeCreditTransferProof
	if proof.ProofType != block.ProofType_PRIM {
		return ErrInvalidProofType
	}
	err = proof.Verify(tx.AddFC.FeeCreditTransfer.UnitId, transferFC, v.verifiers, v.hashAlgorithm)
	if err != nil {
		return errors.Wrap(err, "proof is not valid")
	}
	return nil
}

func (v *DefaultFeeCreditTxValidator) ValidateCloseFC(ctx *CloseFCValidationContext) error {
	if ctx == nil {
		return ErrValidationContextNil
	}
	tx := ctx.Tx
	if tx == nil {
		return ErrTxIsNil
	}

	// P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.Transaction.ClientMetadata.FeeCreditRecordId != nil {
		return ErrRecordIDExists
	}
	if tx.Transaction.FeeProof != nil {
		return ErrFeeProofExists
	}

	// S.N[P.ι] != ⊥ - ι identifies an existing fee credit record
	if ctx.Unit == nil {
		return ErrCloseFCUnitIsNil
	}
	fcr, ok := ctx.Unit.Data.(*txsystem.FeeCreditRecord)
	if !ok {
		return ErrCloseFCInvalidUnitType
	}

	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	if tx.CloseFC.Amount != fcr.Balance {
		return ErrCloseFCInvalidAmount
	}

	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if tx.Transaction.ClientMetadata.MaxFee > fcr.Balance {
		return ErrCloseFCInvalidFee
	}
	return nil
}
