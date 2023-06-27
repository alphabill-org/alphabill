package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
)

func closeFeeCreditTx(tree *rma.Tree, calcFee FeeCalculator, validator *fc.DefaultFeeCreditTxValidator) txsystem.GenericExecuteFunc[transactions.CloseFeeCreditAttributes] {
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
		abFeeBillData := stateDB.GetAlphaBillData(addr)
		var unit *rma.Unit = nil
		if abFeeBillData != nil {
			data := &fc.FeeCreditRecord{
				Balance: weiToAlpha(stateDB.GetBalance(addr)),
				Hash:    abFeeBillData.TxHash,
				Timeout: abFeeBillData.Timeout,
			}
			unit = &rma.Unit{
				Bearer: abFeeBillData.Bearer,
				Data:   data,
			}
		}
		if err = validator.ValidateCloseFC(&fc.CloseFCValidationContext{Tx: tx, Unit: unit}); err != nil {
			return nil, fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		// calculate actual tx fee cost
		sm := &types.ServerMetadata{ActualFee: calcFee()}
		// decrement credit
		stateDB.SubBalance(addr, alphaToWei(attr.Amount))
		return sm, nil
	}
}
