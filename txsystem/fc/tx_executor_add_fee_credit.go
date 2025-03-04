package fc

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (f *FeeCreditModule) executeAddFC(tx *types.TransactionOrder, attr *fc.AddFeeCreditAttributes, _ *fc.AddFeeCreditAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	data := exeCtx.GetData()
	addedFeeCredit := util.BytesToUint64(data[:8])
	latestAdditionTime := util.BytesToUint64(data[8:])
	fee := exeCtx.CalculateCost()
	newBalance := addedFeeCredit - fee

	err := f.state.Apply(unit.IncrCredit(unitID, newBalance, latestAdditionTime))
	// if unable to increment credit because there unit is not found, then create one
	if err != nil && errors.Is(err, avl.ErrNotFound) {
		// add credit
		fcr := fc.NewFeeCreditRecord(newBalance, attr.FeeCreditOwnerPredicate, latestAdditionTime)
		err = f.state.Apply(unit.AddCredit(unitID, fcr))
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

	// target unit is a fee credit record (either new or pre-existing)
	fcr, err := parseFeeCreditRecord(&f.pdr, tx.UnitID, f.feeCreditRecordUnitType, f.state)
	if err != nil && !errors.Is(err, avl.ErrNotFound) {
		return fmt.Errorf("get fcr error: %w", err)
	}
	createFC := errors.Is(err, avl.ErrNotFound)
	transAttr, err := f.checkTransferFC(tx, attr, exeCtx)
	if err != nil {
		return fmt.Errorf("add fee credit validation failed: %w", err)
	}
	// either create new fee credit record or add to existing
	if createFC {
		// if the target does not exist,
		// the identifier must agree with the owner predicate
		fcrID, err := f.NewFeeCreditRecordID(attr.FeeCreditOwnerPredicate, transAttr.LatestAdditionTime)
		if err != nil {
			return fmt.Errorf("failed to create fee credit record id: %w", err)
		}
		if !fcrID.Eq(tx.UnitID) {
			return fmt.Errorf("tx.unitID is not equal to expected fee credit record id (hash of owner predicate), tx.UnitID=%s expected.fcrID=%s", tx.UnitID, fcrID)
		}
		// the transfer counter must not be present
		if transAttr.TargetUnitCounter != nil {
			return errors.New("invalid transferFC target unit counter (target counter must be nil if creating fee credit record for the first time)")
		}
	} else {
		// if the target exists,
		// bill transfer order contains correct target unit counter value
		if transAttr.TargetUnitCounter == nil {
			return errors.New("invalid transferFC target unit counter (target counter must not be nil if updating existing fee credit record)")
		}
		// FCR counter must match transfer counter
		if fcr.GetCounter() != *transAttr.TargetUnitCounter {
			return fmt.Errorf("invalid transferFC target unit counter: transferFC.targetUnitCounter=%d unit.counter=%d", *transAttr.TargetUnitCounter, fcr.GetCounter())
		}
		// the owner predicate matches
		if !bytes.Equal(fcr.OwnerPredicate, attr.FeeCreditOwnerPredicate) {
			return fmt.Errorf("invalid owner predicate: expected=%X actual=%X", fcr.OwnerPredicate, attr.FeeCreditOwnerPredicate)
		}
	}
	// proof of the bill transfer order verifies
	if err = types.VerifyTxProof(attr.FeeCreditTransferProof, f.trustBase, f.hashAlgorithm); err != nil {
		return fmt.Errorf("transFC proof is not valid: %w", err)
	}
	// the owner proof satisfies the bill's owner predicate
	// the attr.FeeCreditOwnerPredicate is verified to be equal to fcr.OwnerPredicate above, for existing unit
	if err = f.execPredicate(attr.FeeCreditOwnerPredicate, authProof.OwnerProof, tx, exeCtx.WithExArg(tx.AuthProofSigBytes)); err != nil {
		return fmt.Errorf("executing fee credit predicate: %w", err)
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
	// bill was transferred to fee credits in this network
	txo, err := transProof.GetTransactionOrderV1()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction order: %w", err)
	}
	if txo.NetworkID != f.pdr.NetworkID {
		return nil, fmt.Errorf("invalid transferFC network identifier %d (expected %d)", txo.NetworkID, f.pdr.NetworkID)
	}
	// bill was transferred in correct partition
	if txo.PartitionID != f.moneyPartitionID {
		return nil, fmt.Errorf("invalid transferFC money partition identifier %d (expected %d)", txo.PartitionID, f.moneyPartitionID)
	}
	if transAttr.TargetPartitionID != f.pdr.PartitionID {
		return nil, fmt.Errorf("invalid transferFC target partition identifier %d (expected %d)", transAttr.TargetPartitionID, f.pdr.PartitionID)
	}
	// bill was transferred to correct target record
	if !bytes.Equal(transAttr.TargetRecordID, tx.UnitID) {
		return nil, fmt.Errorf("invalid transferFC target record id: transferFC.TargetRecordId=%s tx.UnitId=%s", types.UnitID(transAttr.TargetRecordID), tx.UnitID)
	}
	// bill transfer is valid to be used in this block
	if exeCtx.CurrentRound() > transAttr.LatestAdditionTime {
		return nil, fmt.Errorf("invalid transferFC timeout: latestAdditionTime=%d currentRoundNumber=%d", transAttr.LatestAdditionTime, exeCtx.CurrentRound())
	}
	// the transaction fees can’t exceed the transferred value
	feeLimit, ok := util.SafeAdd(tx.MaxFee(), transProof.ActualFee())
	if !ok {
		return nil, errors.New("failed to add tx.maxFee and trans.ActualFee: overflow")
	}
	if feeLimit > transAttr.Amount {
		return nil, fmt.Errorf("invalid transferFC fee: MaxFee+ActualFee=%d transferFC.Amount=%d", feeLimit, transAttr.Amount)
	}

	// find net value of credit and store to execution context to avoid parsing it later
	addedFeeAmount := transAttr.Amount - transProof.ActualFee()
	var data []byte
	data = append(data, util.Uint64ToBytes(addedFeeAmount)...)
	data = append(data, util.Uint64ToBytes(transAttr.LatestAdditionTime)...)
	exeCtx.SetData(data)

	return transAttr, nil
}

func getTransferFC(addFeeCreditProof *types.TxRecordProof) (*fc.TransferFeeCreditAttributes, error) {
	txo, err := addFeeCreditProof.GetTransactionOrderV1()
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction order: %w", err)
	}
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

func (f *FeeCreditModule) NewFeeCreditRecordID(ownerPredicate []byte, timeout uint64) (types.UnitID, error) {
	return f.pdr.ComposeUnitID(types.ShardID{}, f.feeCreditRecordUnitType, fc.PrndSh(ownerPredicate, timeout))
}
