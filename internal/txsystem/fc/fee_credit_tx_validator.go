package fc

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	// DefaultFeeCreditTxValidator default validator for partition specific add/close/lock/unlock fee credit
	// transactions
	DefaultFeeCreditTxValidator struct {
		moneySystemID           []byte
		systemID                []byte
		hashAlgorithm           crypto.Hash
		verifiers               map[string]abcrypto.Verifier
		feeCreditRecordUnitType []byte
	}

	AddFCValidationContext struct {
		Tx                 *types.TransactionOrder
		Unit               *state.Unit
		CurrentRoundNumber uint64
	}

	CloseFCValidationContext struct {
		Tx   *types.TransactionOrder
		Unit *state.Unit
	}

	LockFCValidationContext struct {
		Tx   *types.TransactionOrder
		Attr *transactions.LockFeeCreditAttributes
		Unit *state.Unit
	}

	UnlockFCValidationContext struct {
		Tx   *types.TransactionOrder
		Attr *transactions.UnlockFeeCreditAttributes
		Unit *state.Unit
	}
)

func NewDefaultFeeCreditTxValidator(moneySystemID, systemID []byte, hashAlgorithm crypto.Hash, verifiers map[string]abcrypto.Verifier, feeCreditRecordUnitType []byte) *DefaultFeeCreditTxValidator {
	return &DefaultFeeCreditTxValidator{
		moneySystemID:           moneySystemID,
		systemID:                systemID,
		hashAlgorithm:           hashAlgorithm,
		verifiers:               verifiers,
		feeCreditRecordUnitType: feeCreditRecordUnitType,
	}
}

func (v *DefaultFeeCreditTxValidator) ValidateAddFeeCredit(ctx *AddFCValidationContext) error {
	if err := ctx.isValid(); err != nil {
		return err
	}
	tx := ctx.Tx

	// 10. P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.GetClientFeeCreditRecordID() != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.FeeProof != nil {
		return errors.New("fee tx cannot contain fee authorization proof")
	}

	attr := &transactions.AddFeeCreditAttributes{}
	if err := tx.UnmarshalAttributes(attr); err != nil {
		return fmt.Errorf("failed to unmarshal add fee credit attributes: %w", err)
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

	// 1. ExtrType(P.ι) = fcr – target unit is a fee credit record
	if !tx.UnitID().HasType(v.feeCreditRecordUnitType) {
		return errors.New("invalid unit identifier: type is not fee credit record")
	}

	var fcr *unit.FeeCreditRecord
	if ctx.Unit != nil {
		var ok bool
		fcr, ok = ctx.Unit.Data().(*unit.FeeCreditRecord)
		if !ok {
			return errors.New("invalid unit type: unit is not fee credit record")
		}

		// 2. S.N[P.ι] = ⊥ ∨ S.N[P.ι].φ = P.A.φ – if the target exists, the owner condition matches
		if !bytes.Equal(ctx.Unit.Bearer(), attr.FeeCreditOwnerCondition) {
			return fmt.Errorf("invalid owner condition: expected=%X actual=%X", ctx.Unit.Bearer(), attr.FeeCreditOwnerCondition)
		}
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

	// 7. (S.N[P.ι] = ⊥ ∧ P.A.P.A.η = ⊥) ∨ (S.N[P.ι] != ⊥ ∧ P.A.P.A.η = S.N[P.ι].λ) – bill transfer order contains correct target unit backlink
	if !bytes.Equal(transferTxAttr.TargetUnitBacklink, fcr.GetHash()) {
		return fmt.Errorf("invalid transferFC target unit backlink: transferFC.targetUnitBacklink=%X unit.backlink=%X", transferTxAttr.TargetUnitBacklink, fcr.GetHash())
	}

	// 8. P.A.P.A.tb ≤ t ≤ P.A.P.A.te, where t is the number of the current block being composed – bill transfer is valid to be used in this block
	tb := transferTxAttr.EarliestAdditionTime
	te := transferTxAttr.LatestAdditionTime
	t := ctx.CurrentRoundNumber
	if t < tb || t > te {
		return fmt.Errorf("invalid transferFC timeout: earliest=%d latest=%d current=%d", tb, te, t)
	}

	// 9. P.MC.fa + P.MC.fm ≤ P.A.P.A.v – the transaction fees can’t exceed the transferred value
	feeLimit := tx.Payload.ClientMetadata.MaxTransactionFee + attr.FeeCreditTransfer.ServerMetadata.ActualFee
	if feeLimit > transferTxAttr.Amount {
		return fmt.Errorf("invalid transferFC fee: max_fee+actual_fee=%d transferFC.Amount=%d", feeLimit, transferTxAttr.Amount)
	}

	// 3. VerifyBlockProof(P.A.Π, P.A.P, S.T, S.SD) – proof of the bill transfer order verifies
	err := types.VerifyTxProof(attr.FeeCreditTransferProof, attr.FeeCreditTransfer, v.verifiers, v.hashAlgorithm)
	if err != nil {
		return fmt.Errorf("proof is not valid: %w", err)
	}
	return nil
}

func (v *DefaultFeeCreditTxValidator) ValidateCloseFC(ctx *CloseFCValidationContext) error {
	if err := ctx.isValid(); err != nil {
		return err
	}
	tx := ctx.Tx

	// P.MC.ιf = ⊥ ∧ sf = ⊥ – there’s no fee credit reference or separate fee authorization proof
	if tx.GetClientFeeCreditRecordID() != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.FeeProof != nil {
		return errors.New("fee tx cannot contain fee authorization proof")
	}

	// ExtrType(P.ι) = fcr – target unit is a fee credit record
	if !tx.UnitID().HasType(v.feeCreditRecordUnitType) {
		return errors.New("invalid unit identifier: type is not fee credit record")
	}

	// S.N[P.ι] != ⊥ - ι identifies an existing fee credit record
	fcr, ok := ctx.Unit.Data().(*unit.FeeCreditRecord)
	if !ok {
		return errors.New("unit data is not of type fee credit record")
	}

	// verify the fee credit record is not locked
	if fcr.IsLocked() {
		return errors.New("fee credit record is locked")
	}

	// P.A.v = S.N[ι].b - the amount is the current balance of the record
	closeFCAttributes := &transactions.CloseFeeCreditAttributes{}
	if err := tx.UnmarshalAttributes(closeFCAttributes); err != nil {
		return fmt.Errorf("failed to unmarshal transaction attributes: %w", err)
	}
	if closeFCAttributes.Amount != fcr.Balance {
		return fmt.Errorf("invalid amount: amount=%d fcr.Balance=%d", closeFCAttributes.Amount, fcr.Balance)
	}
	if len(closeFCAttributes.TargetUnitID) == 0 {
		return errors.New("TargetUnitID is empty")
	}
	if len(closeFCAttributes.TargetUnitBacklink) == 0 {
		return errors.New("TargetUnitBacklink is empty")
	}

	// P.MC.fm ≤ S.N[ι].b - the transaction fee can’t exceed the current balance of the record
	if tx.GetClientMaxTxFee() > fcr.Balance {
		return fmt.Errorf("invalid fee: max_fee=%d fcr.Balance=%d", tx.GetClientMaxTxFee(), fcr.Balance)
	}
	return nil
}

func (v *DefaultFeeCreditTxValidator) ValidateLockFC(ctx *LockFCValidationContext) error {
	// nil checks
	if err := ctx.isValid(); err != nil {
		return err
	}
	// tx unit id identifies an existing fee credit record
	fcr, err := v.parseFeeCreditRecord(ctx.Tx, ctx.Unit)
	if err != nil {
		return fmt.Errorf("fee credit record not found for given unit id: %w", err)
	}
	// verify fcr is not already locked
	if fcr.IsLocked() {
		return errors.New("fee credit record is already locked")
	}
	// verify lock status is non-zero i.e. "locked"
	if ctx.Attr.LockStatus == 0 {
		return errors.New("lock status must be non-zero value")
	}
	return v.validateLockTxs(ctx.Tx, fcr, ctx.Attr.Backlink)
}

func (v *DefaultFeeCreditTxValidator) ValidateUnlockFC(ctx *UnlockFCValidationContext) error {
	// nil checks
	if err := ctx.isValid(); err != nil {
		return err
	}
	// tx unit id identifies an existing fee credit record
	fcr, err := v.parseFeeCreditRecord(ctx.Tx, ctx.Unit)
	if err != nil {
		return fmt.Errorf("fee credit record not found for given unit id: %w", err)
	}
	// verify fcr is not already unlocked
	if !fcr.IsLocked() {
		return errors.New("fee credit record is already unlocked")
	}
	return v.validateLockTxs(ctx.Tx, fcr, ctx.Attr.Backlink)
}

func (v *DefaultFeeCreditTxValidator) validateLockTxs(tx *types.TransactionOrder, fcr *unit.FeeCreditRecord, txBacklink []byte) error {
	// the transaction follows the previous valid transaction with the record
	if !bytes.Equal(txBacklink, fcr.GetHash()) {
		return fmt.Errorf("the transaction backlink does not match with unit transaction hash: "+
			"got %x expected %x", txBacklink, fcr.GetHash())
	}

	// the transaction fees can’t exceed the fee credit record balance
	if tx.GetClientMaxTxFee() > fcr.Balance {
		return fmt.Errorf("max fee cannot exceed fee credit record balance: tx.maxFee=%d fcr.Balance=%d",
			tx.GetClientMaxTxFee(), fcr.Balance)
	}

	// there’s no fee credit reference or separate fee authorization proof
	if tx.GetClientFeeCreditRecordID() != nil {
		return errors.New("fee tx cannot contain fee credit reference")
	}
	if tx.FeeProof != nil {
		return errors.New("fee tx cannot contain fee authorization proof")
	}
	return nil
}

func (c *AddFCValidationContext) isValid() error {
	if c == nil {
		return errors.New("validation context is nil")
	}
	if c.Tx == nil {
		return errors.New("tx is nil")
	}
	if c.Tx.Payload == nil {
		return errors.New("tx payload is nil")
	}
	if c.Tx.Payload.ClientMetadata == nil {
		return errors.New("tx client metadata is nil")
	}
	return nil
}

func (c *CloseFCValidationContext) isValid() error {
	if c == nil {
		return errors.New("validation context is nil")
	}
	if c.Unit == nil {
		return errors.New("unit is nil")
	}
	if c.Tx == nil {
		return errors.New("tx is nil")
	}
	if c.Tx.Payload == nil {
		return errors.New("tx payload is nil")
	}
	if c.Tx.Payload.ClientMetadata == nil {
		return errors.New("tx client metadata is nil")
	}
	return nil
}

func (c *LockFCValidationContext) isValid() error {
	if c == nil {
		return errors.New("validation context is nil")
	}
	if c.Unit == nil {
		return errors.New("unit is nil")
	}
	if c.Tx == nil {
		return errors.New("tx is nil")
	}
	if c.Attr == nil {
		return errors.New("tx attributes is nil")
	}
	return nil
}

func (c *UnlockFCValidationContext) isValid() error {
	if c == nil {
		return errors.New("validation context is nil")
	}
	if c.Unit == nil {
		return errors.New("unit is nil")
	}
	if c.Tx == nil {
		return errors.New("tx is nil")
	}
	if c.Attr == nil {
		return errors.New("tx attributes is nil")
	}
	return nil
}

func (v *DefaultFeeCreditTxValidator) parseFeeCreditRecord(tx *types.TransactionOrder, u *state.Unit) (*unit.FeeCreditRecord, error) {
	if !tx.UnitID().HasType(v.feeCreditRecordUnitType) {
		return nil, errors.New("invalid unit identifier: type is not fee credit record")
	}
	var fcr *unit.FeeCreditRecord
	var ok bool
	fcr, ok = u.Data().(*unit.FeeCreditRecord)
	if !ok {
		return nil, errors.New("invalid unit type: unit is not fee credit record")
	}
	return fcr, nil
}
