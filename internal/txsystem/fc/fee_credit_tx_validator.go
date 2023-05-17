package fc

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
)

var (
	ErrValidationContextNil = errors.New("validation context is nil")
	ErrTxIsNil              = errors.New("tx is nil")
	ErrTxPayloadIsNil       = errors.New("tx payload is nil")
	ErrRecordIDExists       = errors.New("fee tx cannot contain fee credit reference")
	ErrFeeProofExists       = errors.New("fee tx cannot contain fee authorization proof")
	ErrInvalidProofType     = errors.New("invalid proof type")

	// add fee credit errors
	ErrAddFCInvalidOwnerCondition = errors.New("addFC: invalid owner condition")
	ErrAddFCInvalidSystemID       = errors.New("addFC: invalid transferFC system identifier")
	ErrAddFCInvalidTargetSystemID = errors.New("addFC: invalid transferFC target system identifier")
	ErrAddFCInvalidTargetRecordID = errors.New("addFC: invalid transferFC target record id")
	ErrAddFCInvalidNonce          = errors.New("addFC: invalid transferFC nonce")
	ErrAddFCInvalidTimeout        = errors.New("addFC: invalid transferFC timeout")
	ErrAddFCInvalidTxFee          = errors.New("addFC: invalid transferFC fee")

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
		Tx                 *types.TransactionOrder
		Unit               *rma.Unit
		CurrentRoundNumber uint64
	}

	CloseFCValidationContext struct {
		Tx   *types.TransactionOrder
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
		return ErrValidationContextNil
	}
	tx := ctx.Tx
	if tx == nil {
		return ErrTxIsNil
	}

	if tx.Payload == nil {
		return ErrTxPayloadIsNil
	}

	// 10. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.Payload.ClientMetadata.FeeCreditRecordID != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}

	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	if tx.PayloadType() != transactions.PayloadTypeAddFeeCredit {
		return fmt.Errorf("invalid transation payload type: %s", tx.PayloadType())
	}
	attr := &transactions.AddFeeCreditAttributes{}
	if err := tx.UnmarshalAttributes(attr); err != nil {
		return fmt.Errorf("failed to unmarshal add fee credit attributes: %w", err)
	}

	// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner condition matches
	unit := ctx.Unit
	if unit != nil && !bytes.Equal(unit.Bearer, attr.FeeCreditOwnerCondition) {
		return ErrAddFCInvalidOwnerCondition
	}

	// 4. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	transferTx := attr.FeeCreditTransfer.TransactionOrder
	if !bytes.Equal(transferTx.SystemID(), v.moneySystemID) {
		return ErrAddFCInvalidSystemID
	}

	if transferTx.PayloadType() != transactions.PayloadTypeTransferFeeCredit {
		return fmt.Errorf("invalid transfer fee credit transation payload type: %s", transferTx.PayloadType())
	}

	transferTxAttr := &transactions.TransferFeeCreditAttributes{}
	if err := transferTx.UnmarshalAttributes(transferTxAttr); err != nil {
		return fmt.Errorf("failed to unmarshal transfer fee credit attributes: %w", err)
	}

	// 5. P.A.P.A.α = P.α – bill was transferred to fee credits for this system
	if !bytes.Equal(transferTxAttr.TargetSystemIdentifier, v.systemID) {
		return ErrAddFCInvalidTargetSystemID
	}

	// 6. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(transferTxAttr.TargetRecordID, tx.UnitID()) {
		return ErrAddFCInvalidTargetRecordID
	}

	// 7. (S.N[P.ι] = ⊥ ∧ P.A.P.A.η = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P.A.P.A.η = S.N[P.ι].λ) – bill transfer order contains correct nonce
	var backlink []byte
	if unit != nil {
		backlink = unit.StateHash
	}
	if !bytes.Equal(transferTxAttr.Nonce, backlink) {
		return ErrAddFCInvalidNonce
	}

	// 8. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	tb := transferTxAttr.EarliestAdditionTime
	te := transferTxAttr.LatestAdditionTime
	t := ctx.CurrentRoundNumber
	if t < tb || t > te {
		return ErrAddFCInvalidTimeout
	}

	// 9. P.MC.fm ≤ P.A.P.A.v – the transaction fee can’t exceed the transferred value
	if tx.Payload.ClientMetadata.MaxTransactionFee > transferTxAttr.Amount {
		return ErrAddFCInvalidTxFee
	}

	// 3. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies

	proof := attr.FeeCreditTransferProof
	err := types.VerifyTxProof(proof, attr.FeeCreditTransfer, v.verifiers, v.hashAlgorithm)
	if err != nil {
		return fmt.Errorf("invalid proof: %w", err)
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
	if tx.Payload == nil {
		return ErrTxPayloadIsNil
	}
	if tx.PayloadType() != transactions.PayloadTypeCloseFeeCredit {
		return fmt.Errorf("invalid transation payload type: %s", tx.PayloadType())
	}

	// P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.Payload.ClientMetadata.FeeCreditRecordID != nil {
		return ErrRecordIDExists
	}
	if tx.FeeProof != nil {
		return ErrFeeProofExists
	}

	// S.N[P.ι] != ⊥ - ι identifies an existing fee credit record
	if ctx.Unit == nil {
		return ErrCloseFCUnitIsNil
	}
	fcr, ok := ctx.Unit.Data.(*FeeCreditRecord)
	if !ok {
		return ErrCloseFCInvalidUnitType
	}

	closFCAttributes := &transactions.CloseFeeCreditAttributes{}
	if err := tx.UnmarshalAttributes(closFCAttributes); err != nil {
		return fmt.Errorf("failed to unmarshal transaction attributes: %w", err)
	}

	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	if closFCAttributes.Amount != fcr.Balance {
		return ErrCloseFCInvalidAmount
	}

	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if tx.Payload.ClientMetadata.MaxTransactionFee > fcr.Balance {
		return ErrCloseFCInvalidFee
	}
	return nil
}
