package money

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *Module) executeSwapTx(tx *types.TransactionOrder, _ *money.SwapDCAttributes, _ *money.SwapDCAuthProof, exeCtx txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	// get swap amount from execution context to avoid parsing swap tx attributes twice
	swapAmount := util.BytesToUint64(exeCtx.GetData())

	// N[T.ιDC].D.v ← N[T.ιDC].D.v − v – decrease the DC money supply
	updateDCMoneySupplyFn := state.UpdateUnitData(DustCollectorMoneySupplyID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", DustCollectorMoneySupplyID)
			}
			bd.Value -= swapAmount
			return bd, nil
		},
	)
	// v ← T′1.A.v + ... + T′m.A.v – the value to join to target bill
	// N[T.ι].D.v ← N[T.ι].D.v + v – increase the value of ι
	// N[T.ι].D.ℓ ← 0
	// N[T.ι].D.c ← N[T.ι].D.c + 1
	updateTargetUnitFn := state.UpdateUnitData(tx.UnitID,
		func(data types.UnitData) (types.UnitData, error) {
			bd, ok := data.(*money.BillData)
			if !ok {
				return nil, fmt.Errorf("unit %v does not contain bill data", tx.UnitID)
			}
			bd.Value += swapAmount
			bd.Counter += 1
			bd.Locked = 0
			return bd, nil
		})
	if err := m.state.Apply(updateDCMoneySupplyFn, updateTargetUnitFn); err != nil {
		return nil, fmt.Errorf("unit update failed: %w", err)
	}
	return &types.ServerMetadata{
		TargetUnits:      []types.UnitID{tx.UnitID, DustCollectorMoneySupplyID},
		SuccessIndicator: types.TxStatusSuccessful,
	}, nil
}

func (m *Module) validateSwapTx(tx *types.TransactionOrder, attr *money.SwapDCAttributes, authProof *money.SwapDCAuthProof, exeCtx txtypes.ExecutionContext) error {
	// transaction unit id identifies an existing bill
	unitData, err := m.state.GetUnit(tx.UnitID, false)
	if err != nil {
		return fmt.Errorf("target unit error: %w", err)
	}
	// if unit exists (get returns no error) it must not be nil, if unit data is nil then this type assertion will fail
	billData, ok := unitData.Data().(*money.BillData)
	if !ok {
		return fmt.Errorf("target unit invalid data type")
	}

	// the owner proof satisfies the bill's owner predicate
	if err = m.execPredicate(billData.OwnerPredicate, authProof.OwnerProof, tx.AuthProofSigBytes, exeCtx); err != nil {
		return fmt.Errorf("swap transaction predicate validation failed: %w", err)
	}

	// verify individual dust transfers (without proofs, proofs are check as the last step)
	dcSum, err := m.verifyDustTransfers(attr.DustTransferProofs, tx.UnitID, billData.Counter)
	if err != nil {
		return fmt.Errorf("dust transaction verification failed: %w", err)
	}

	// there is sufficient DC-money supply
	dcMoneySupply, err := m.state.GetUnit(DustCollectorMoneySupplyID, false)
	if err != nil {
		return fmt.Errorf("DC-money supply unit error: %w", err)
	}
	dcMoneySupplyBill, ok := dcMoneySupply.Data().(*money.BillData)
	if !ok {
		return errors.New("DC-money supply invalid data type")
	}
	if dcMoneySupplyBill.Value < dcSum {
		return errors.New("insufficient DC-money supply")
	}

	// verify dust transfer proofs
	for i, proof := range attr.DustTransferProofs {
		if err = types.VerifyTxProof(proof, m.trustBase, m.hashAlgorithm); err != nil {
			return fmt.Errorf("dust transfer proof is not valid at index %d: %w", i, err)
		}
	}

	// add DC-sum to the execution context to avoid parsing the dust transfers again at execution step
	exeCtx.SetData(util.Uint64ToBytes(dcSum))

	return nil
}

func (m *Module) verifyDustTransfers(dustTransferProofs []*types.TxRecordProof, targetUnitID types.UnitID, targetUnitCounter uint64) (uint64, error) {
	var dcSum uint64
	var prevDcProof *types.TxRecordProof
	for i, dcProof := range dustTransferProofs {
		if i > 0 {
			prevDcProof = dustTransferProofs[i-1]
		}
		dcVal, err := m.verifyDustTransfer(dcProof, prevDcProof, targetUnitID, targetUnitCounter)
		if err != nil {
			return 0, fmt.Errorf("failed to verify dust transfer at index %d: %w", i, err)
		}
		var ok bool
		dcSum, ok = util.SafeAdd(dcSum, dcVal)
		if !ok {
			return 0, fmt.Errorf("dust transfer transaction sum overflow")
		}
	}
	return dcSum, nil
}

func (m *Module) verifyDustTransfer(dcProof *types.TxRecordProof, prevDustTransfer *types.TxRecordProof, targetUnitID types.UnitID, targetUnitCounter uint64) (uint64, error) {
	if err := dcProof.IsValid(); err != nil {
		return 0, err
	}
	dcTxo := dcProof.TransactionOrder()
	var dcAttr money.TransferDCAttributes
	if err := dcTxo.UnmarshalAttributes(&dcAttr); err != nil {
		return 0, fmt.Errorf("invalid TransferDC attributes: %w", err)
	}
	// transfers were in this network
	if dcTxo.NetworkID != m.networkID {
		return 0, fmt.Errorf("dust transfer invalid network: expected %d vs provided %d", m.networkID, dcTxo.SystemID)
	}
	// transfers were in the money partition
	if dcTxo.SystemID != m.systemID {
		return 0, fmt.Errorf("dust transfer system id is not money partition system id: expected %d vs provided %d",
			m.systemID, dcTxo.SystemID)
	}
	// bills were transferred to DC
	if dcTxo.Type != money.TransactionTypeTransDC {
		return 0, fmt.Errorf("invalid transfer DC transaction type: %d", dcTxo.Type)
	}
	// transfer orders are listed in strictly increasing order of bill identifiers
	// (this ensures that no source bill can be included multiple times
	if prevDustTransfer != nil && bytes.Compare(dcTxo.UnitID, prevDustTransfer.TxRecord.UnitID()) != 1 {
		return 0, errors.New("dust transfer orders are not listed in strictly increasing order of bill identifiers")
	}
	// bill transfer orders contain correct target unit ids
	if !bytes.Equal(dcAttr.TargetUnitID, targetUnitID) {
		return 0, errors.New("dust transfer order target unit id is not equal to swap transaction unit id")
	}
	// bill transfer orders contain correct target counter values
	if dcAttr.TargetUnitCounter != targetUnitCounter {
		return 0, fmt.Errorf("dust transfer target counter is not equal to target unit counter: "+
			"expected %d vs provided %d", targetUnitCounter, dcAttr.TargetUnitCounter)
	}
	// proofs are verifier later
	return dcAttr.Value, nil
}
