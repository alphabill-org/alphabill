package evm

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	"github.com/alphabill-org/alphabill/validator/internal/types"
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

func addFeeCreditTx(s *state.State, hashAlgorithm crypto.Hash, calcFee FeeCalculator, validator *fc.DefaultFeeCreditTxValidator) txsystem.GenericExecuteFunc[transactions.AddFeeCreditAttributes] {
	return func(tx *types.TransactionOrder, attr *transactions.AddFeeCreditAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		pubKey, err := templates.ExtractPubKey(tx.OwnerProof)
		if err != nil {
			return nil, fmt.Errorf("failed to extract public key from fee credit owner proof")
		}
		address, err := generateAddress(pubKey)
		if err != nil {
			return nil, fmt.Errorf("failed to extract address from public key bytes, %w", err)
		}
		// unit id is ethereum address
		unitID := address.Bytes()
		u, _ := s.GetUnit(unitID, false)
		// hack to be able to use a common validator for now
		var feeCreditRecordUnit *state.Unit = nil
		// unit exists, and it has unit has AB fcr link set then simulate FCR bill
		if u != nil && u.Data().(*statedb.StateObject).AlphaBill != nil {
			stateObj := u.Data().(*statedb.StateObject)
			data := &unit.FeeCreditRecord{
				Balance:  weiToAlpha(stateObj.Account.Balance),
				Backlink: stateObj.AlphaBill.TxHash,
				Timeout:  stateObj.AlphaBill.Timeout,
			}
			feeCreditRecordUnit = state.NewUnit(
				u.Bearer(),
				data)
		}
		if err = validator.ValidateAddFeeCredit(&fc.AddFCValidationContext{
			Tx:                 tx,
			Unit:               feeCreditRecordUnit,
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
		v := transferFc.Amount - attr.FeeCreditTransfer.ServerMetadata.ActualFee - fee
		// if unit exists update balance and alphabill free credit link data
		var action []state.Action
		if u == nil {
			action = append(action, statedb.CreateAccountAndAddCredit(address, attr.FeeCreditOwnerCondition, alphaToWei(v), transferFc.LatestAdditionTime+1, tx.Hash(hashAlgorithm)))
		} else {
			// update account balance and AB FCR bill backlink and timeout
			action = append(action, statedb.UpdateEthAccountAddCredit(unitID, alphaToWei(v), transferFc.LatestAdditionTime+1, tx.Hash(hashAlgorithm)))
			// also update owner condition as the account may have been created by a transfer or smart contract without one
			action = append(action, state.SetOwner(unitID, attr.FeeCreditOwnerCondition))
		}
		if err = s.Apply(action...); err != nil {
			return nil, fmt.Errorf("addFC state update failed: %w", err)
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}
