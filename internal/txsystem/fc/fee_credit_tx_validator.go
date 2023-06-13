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
		return errors.New("validation context is nil")
	}
	tx := ctx.Tx
	if tx == nil {
		return errors.New("tx is nil")
	}

	if tx.Payload == nil {
		return errors.New("tx payload is nil")
	}

	// 10. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.GetClientFeeCreditRecordID() != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.FeeProof != nil {
		return errors.New("fee tx cannot contain fee authorization proof")
	}

	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	// TODO

	attr := &transactions.AddFeeCreditAttributes{}
	if err := tx.UnmarshalAttributes(attr); err != nil {
		return fmt.Errorf("failed to unmarshal add fee credit attributes: %w", err)
	}

	// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner condition matches
	unit := ctx.Unit
	if unit != nil && !bytes.Equal(unit.Bearer, attr.FeeCreditOwnerCondition) {
		return fmt.Errorf("invalid owner condition: expected=%X actual=%X", unit.Bearer, attr.FeeCreditOwnerCondition)
	}

	// 4. P.A.P.α = P.αmoney ∧ P.A.P.τ = transFC – bill was transferred to fee credits
	transferTx := attr.FeeCreditTransfer.TransactionOrder
	if !bytes.Equal(transferTx.SystemID(), v.moneySystemID) {
		return errors.New("addFC: invalid transferFC system identifier")
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
		return fmt.Errorf("invalid transferFC target system identifier: expected_target_system_id: %X actual_target_system_id=%X", v.systemID, transferTxAttr.TargetSystemIdentifier)
	}

	// 6. P.A.P.A.ιf = P.ι – bill was transferred to fee credits of the target record
	if !bytes.Equal(transferTxAttr.TargetRecordID, tx.UnitID()) {
		return fmt.Errorf("invalid transferFC target record id: transferFC.TargetRecordId=%X tx.UnitId=%X", transferTxAttr.TargetRecordID, tx.UnitID())
	}

	// 7. (S.N[P.ι] = ⊥ ∧ P.A.P.A.η = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P.A.P.A.η = S.N[P.ι].λ) – bill transfer order contains correct nonce
	var backlink []byte
	if unit != nil {
		backlink = unit.Data.(*FeeCreditRecord).Hash
	}
	if !bytes.Equal(transferTxAttr.Nonce, backlink) {
		return fmt.Errorf("invalid transferFC nonce: transferFC.nonce=%X unit.backlink=%X", transferTxAttr.Nonce, backlink)
	}

	// 8. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	tb := transferTxAttr.EarliestAdditionTime
	te := transferTxAttr.LatestAdditionTime
	t := ctx.CurrentRoundNumber
	if t < tb || t > te {
		return fmt.Errorf("invalid transferFC timeout: earliest=%d latest=%d current=%d", tb, te, t)
	}

	// 9. P.MC.fm ≤ P.A.P.A.v – the transaction fee can’t exceed the transferred value
	if tx.Payload.ClientMetadata.MaxTransactionFee > transferTxAttr.Amount {
		return fmt.Errorf("invalid transferFC fee: max_fee=%d transferFC.Amount=%d", tx.Payload.ClientMetadata.MaxTransactionFee, transferTxAttr.Amount)
	}

	// 3. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	proof := attr.FeeCreditTransferProof
	err := types.VerifyTxProof(proof, attr.FeeCreditTransfer, v.verifiers, v.hashAlgorithm)
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
	if tx.Payload == nil {
		return errors.New("tx payload is nil")
	}

	// P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.GetClientFeeCreditRecordID() != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.FeeProof != nil {
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
	closFCAttributes := &transactions.CloseFeeCreditAttributes{}
	if err := tx.UnmarshalAttributes(closFCAttributes); err != nil {
		return fmt.Errorf("failed to unmarshal transaction attributes: %w", err)
	}
	if closFCAttributes.Amount != fcr.Balance {
		return fmt.Errorf("invalid amount: amount=%d fcr.Balance=%d", closFCAttributes.Amount, fcr.Balance)
	}

	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if tx.Payload.ClientMetadata.MaxTransactionFee > fcr.Balance {
		return fmt.Errorf("invalid fee: max_fee=%d fcr.Balance=%d", tx.Payload.ClientMetadata.MaxTransactionFee, fcr.Balance)
	}
	return nil
}
