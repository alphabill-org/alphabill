package evm

import (
	"crypto"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
)

func closeFeeCreditTx(tree *state.State, hashAlgorithm crypto.Hash, calcFee FeeCalculator, validator *fc.DefaultFeeCreditTxValidator, log *slog.Logger) txsystem.GenericExecuteFunc[transactions.CloseFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.CloseFeeCreditAttributes, ctx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		pubKey, err := predicates.ExtractPubKey(tx.OwnerProof)
		if err != nil {
			return nil, fmt.Errorf("failed to extract public key from fee credit owner proof")
		}
		addr, err := generateAddress(pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to extract address from public key bytes, %w", err)
		}
		txHash := tx.Hash(hashAlgorithm)
		unitID := addr.Bytes()
		u, _ := tree.GetUnit(unitID, false)
		// hack to be able to use a common validator for now
		var feeCreditRecordUnit *state.Unit = nil
		if u != nil {
			stateObj := u.Data().(*statedb.StateObject)
			data := &unit.FeeCreditRecord{
				Balance:  weiToAlpha(stateObj.Account.Balance),
				Backlink: txHash,
				Timeout:  stateObj.AlphaBill.Timeout,
			}
			feeCreditRecordUnit = state.NewUnit(
				u.Bearer(),
				data,
			)
		}
		if err = validator.ValidateCloseFC(&fc.CloseFCValidationContext{Tx: tx, Unit: feeCreditRecordUnit}); err != nil {
			return nil, fmt.Errorf("closeFC: tx validation failed: %w", err)
		}

		// decrement credit and update AB FCR bill backlink
		if err = tree.Apply(statedb.UpdateEthAccountCloseCredit(unitID, alphaToWei(attr.Amount), txHash)); err != nil {
			return nil, fmt.Errorf("closeFC state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: calcFee(), TargetUnits: []types.UnitID{addr.Bytes()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}
