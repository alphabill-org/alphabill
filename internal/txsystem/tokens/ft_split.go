package tokens

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/holiman/uint256"
)

func handleSplitFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*splitFungibleTokenWrapper] {
	return func(tx *splitFungibleTokenWrapper, currentBlockNr uint64) error {
		logger.Debug("Processing Split Fungible Token tx: %v", tx)
		if err := validateSplitFungibleToken(tx, options.state); err != nil {
			return fmt.Errorf("invalid split fungible token tx: %w", err)
		}
		u, err := options.state.GetUnit(tx.UnitID())
		if err != nil {
			return err
		}
		d := u.Data.(*fungibleTokenData)
		// add new token unit
		newTokenID := txutil.SameShardID(tx.UnitID(), tx.HashForIDCalculation(options.hashAlgorithm))
		logger.Debug("Adding a fungible token with ID %v", newTokenID)
		txHash := tx.Hash(options.hashAlgorithm)
		fee := options.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})
		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, txHash),
			rma.AddItem(newTokenID,
				tx.attributes.NewBearer,
				&fungibleTokenData{
					tokenType: d.tokenType,
					value:     tx.attributes.TargetValue,
					t:         currentBlockNr,
					backlink:  txHash,
				}, txHash),
			rma.UpdateData(tx.UnitID(),
				func(data rma.UnitData) (newData rma.UnitData) {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						// No change in case of incorrect data type.
						return data
					}
					return &fungibleTokenData{
						tokenType: d.tokenType,
						value:     d.value - tx.attributes.TargetValue,
						t:         currentBlockNr,
						backlink:  txHash,
					}
				}, txHash))
	}
}

func validateSplitFungibleToken(tx *splitFungibleTokenWrapper, state *rma.Tree) error {
	d, err := getFungibleTokenData(tx.UnitID(), state)
	if err != nil {
		return err
	}
	if d.value < tx.attributes.TargetValue {
		return errors.Errorf("invalid token value: max allowed %v, got %v", d.value, tx.attributes.TargetValue)
	}
	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		state,
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, tx.InvariantPredicateSignatures(), tx.SigBytes())
}
