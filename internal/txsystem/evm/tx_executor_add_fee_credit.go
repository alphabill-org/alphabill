package evm

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
)

func getTransferPayloadAttributes(transfer *types.TransactionRecord) (*transactions.TransferFeeCreditAttributes, error) {
	if transfer == nil {
		return nil, fmt.Errorf("transfer record is nil")
	}
	transferPayload := &transactions.TransferFeeCreditAttributes{}
	if err := transfer.TransactionOrder.UnmarshalAttributes(transferPayload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transfer payload: %w", err)
	}
	return transferPayload, nil
}

func addFeeCreditTx(tree *rma.Tree, hashAlgorithm crypto.Hash, calcFee FeeCalculator, validator *fc.DefaultFeeCreditTxValidator) txsystem.GenericExecuteFunc[transactions.AddFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.AddFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing addFC %v", tx)
		stateDB := statedb.NewStateDB(tree)
		pubKey, err := script.ExtractPubKeyFromPredicateArgument(tx.OwnerProof)
		if err != nil {
			return nil, fmt.Errorf("failed to extract public key from fee credit owner proof")
		}
		addr, err := generateAddress(pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to extract address from public key bytes, %w", err)
		}
		feeData := stateDB.GetFeeData(addr)
		// hack to be able to use a common validator for now
		var unit *rma.Unit = nil
		if feeData != nil {
			data := &fc.FeeCreditRecord{
				Balance: weiToAlpha(stateDB.GetBalance(addr)),
				Hash:    feeData.TxHash,
				Timeout: feeData.Timeout,
			}
			unit = &rma.Unit{
				Bearer: feeData.Bearer,
				Data:   data,
			}
		}
		if err = validator.ValidateAddFeeCredit(&fc.AddFCValidationContext{
			Tx:                 tx,
			Unit:               unit,
			CurrentRoundNumber: currentBlockNumber,
		}); err != nil {
			return nil, fmt.Errorf("addFC tx validation failed: %w", err)
		}
		fee := calcFee()
		// find net value of credit
		transferFc, err := getTransferPayloadAttributes(attr.FeeCreditTransfer)
		if err != nil {
			return nil, err
		}
		v := transferFc.Amount - fee
		stateDB.CreateAccount(addr)
		stateDB.AddBalance(addr, alphaToWei(v))
		// This is an EOA account so in theory it does not have any other storage so there should be no conflicts
		// update fee data
		stateDB.SetFeeData(addr, &statedb.FeeBillData{
			Bearer:  attr.FeeCreditOwnerCondition,
			UnitID:  tx.UnitID(),
			TxHash:  tx.Hash(hashAlgorithm),
			Timeout: transferFc.LatestAdditionTime + 1,
		})
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}
