package tokens

import (
	"bytes"
	goerrors "errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
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

		fee := options.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		// calculate hash after setting server metadata
		txHash := tx.Hash(options.hashAlgorithm)

		// update state
		// disable fee handling if fee is calculated to 0 (used to temporarily disable fee handling, can be removed after all wallets are updated)
		var fcFunc rma.Action
		if options.feeCalculator() == 0 {
			fcFunc = func(tree *rma.Tree) error {
				return nil
			}
		} else {
			fcrID := tx.transaction.GetClientFeeCreditRecordID()
			fcFunc = fc.DecrCredit(fcrID, fee, txHash)
		}

		return options.state.AtomicUpdate(
			fcFunc,
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
	bearer, d, err := getFungibleTokenData(tx.UnitID(), state)
	if err != nil {
		return err
	}
	if tx.TargetValue() == 0 {
		return goerrors.New("when splitting a token the value assigned to the new token must be greater than zero")
	}
	if tx.RemainingValue() == 0 {
		return goerrors.New("when splitting a token the remaining value of the token must be greater than zero")
	}

	if d.value < tx.attributes.TargetValue {
		return fmt.Errorf("invalid token value: max allowed %v, got %v", d.value, tx.attributes.TargetValue)
	}
	if rm := d.value - tx.TargetValue(); tx.RemainingValue() != rm {
		return goerrors.New("remaining value must equal to the original value minus target value")
	}

	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	tokenTypeID := util.Uint256ToBytes(d.tokenType)
	if !bytes.Equal(tx.TypeID(), tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%X', got '%X'", tokenTypeID, tx.TypeID())
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
	return verifyOwnership(bearer, predicates, tx)
}
