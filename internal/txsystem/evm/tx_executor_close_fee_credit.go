package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
)

func closeFeeCreditTx(tree *state.State, calcFee FeeCalculator, validator *fc.DefaultFeeCreditTxValidator) txsystem.GenericExecuteFunc[transactions.CloseFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.CloseFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing closeFC %v", tx)
		stateDB := statedb.NewStateDB(tree)
		pubKey, err := script.ExtractPubKeyFromPredicateArgument(tx.OwnerProof)
		if err != nil {
			return nil, fmt.Errorf("failed to extract public key from fee credit owner proof")
		}
		addr, err := generateAddress(pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to extract address from public key bytes, %w", err)
		}
		unitID := addr.Bytes()
		u, _ := tree.GetUnit(unitID, false)
		// hack to be able to use a common validator for now
		var feeCreditRecordUnit *state.Unit = nil
		if u != nil {
			stateObj := u.Data().(*statedb.StateObject)
			data := &unit.FeeCreditRecord{
				Balance: weiToAlpha(stateObj.Account.Balance),
				Hash:    stateObj.AlphaBill.TxHash,
				Timeout: stateObj.AlphaBill.Timeout,
			}
			feeCreditRecordUnit = state.NewUnit(
				u.Bearer(),
				data,
			)
		}
		if err = validator.ValidateCloseFC(&fc.CloseFCValidationContext{Tx: tx, Unit: feeCreditRecordUnit}); err != nil {
			return nil, fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		// decrement credit
		stateDB.SubBalance(addr, alphaToWei(attr.Amount))
		// calculate actual tx fee cost
		return &types.ServerMetadata{ActualFee: calcFee(), TargetUnits: []types.UnitID{addr.Bytes()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}
