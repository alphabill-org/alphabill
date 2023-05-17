package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleSplitFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[SplitFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Split Fungible Token tx: %v", tx)
		if err := validateSplitFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid split fungible token tx: %w", err)
		}
		unitID := util.BytesToUint256(tx.UnitID())
		u, err := options.state.GetUnit(unitID)
		if err != nil {
			return nil, err
		}
		d := u.Data.(*fungibleTokenData)
		// add new token unit
		newTokenID := txutil.SameShardID(unitID, hashForIDCalculation(tx, options.hashAlgorithm))
		logger.Debug("Adding a fungible token with ID %v", newTokenID)

		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		txHash := tx.Hash(options.hashAlgorithm)

		// update state
		// disable fee handling if fee is calculated to 0 (used to temporarily disable fee handling, can be removed after all wallets are updated)
		var fcFunc rma.Action
		if options.feeCalculator() == 0 {
			fcFunc = func(tree *rma.Tree) error {
				return nil
			}
		} else {
			fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
			fcFunc = fc.DecrCredit(fcrID, fee, txHash)
		}

		if err = options.state.AtomicUpdate(
			fcFunc,
			rma.AddItem(newTokenID,
				attr.NewBearer,
				&fungibleTokenData{
					tokenType: d.tokenType,
					value:     attr.TargetValue,
					t:         currentBlockNr,
					backlink:  txHash,
				}, txHash),
			rma.UpdateData(unitID,
				func(data rma.UnitData) (newData rma.UnitData) {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						// No change in case of incorrect data type.
						return data
					}
					return &fungibleTokenData{
						tokenType: d.tokenType,
						value:     d.value - attr.TargetValue,
						t:         currentBlockNr,
						backlink:  txHash,
					}
				}, txHash)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func hashForIDCalculation(tx *types.TransactionOrder, hashFunc crypto.Hash) []byte {
	hasher := hashFunc.New()
	idBytes := util.BytesToUint256(tx.UnitID()).Bytes32()
	hasher.Write(idBytes[:])
	hasher.Write(tx.Payload.Attributes)
	hasher.Write(util.Uint64ToBytes(tx.Timeout()))
	return hasher.Sum(nil)
}

func validateSplitFungibleToken(tx *types.TransactionOrder, attr *SplitFungibleTokenAttributes, state *rma.Tree) error {
	bearer, d, err := getFungibleTokenData(util.BytesToUint256(tx.UnitID()), state)
	if err != nil {
		return err
	}
	if attr.TargetValue == 0 {
		return errors.New("when splitting a token the value assigned to the new token must be greater than zero")
	}
	if attr.RemainingValue == 0 {
		return errors.New("when splitting a token the remaining value of the token must be greater than zero")
	}

	if d.value < attr.TargetValue {
		return fmt.Errorf("invalid token value: max allowed %v, got %v", d.value, attr.TargetValue)
	}
	if rm := d.value - attr.TargetValue; attr.RemainingValue != rm {
		return errors.New("remaining value must equal to the original value minus target value")
	}

	if !bytes.Equal(d.backlink, attr.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
	}
	tokenTypeID := util.Uint256ToBytes(d.tokenType)
	if !bytes.Equal(attr.Type, tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%X', got '%X'", tokenTypeID, attr.Type)
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
	return verifyOwnership(bearer, predicates, TokenOwnershipProver{tx: tx, invariantPredicateSignatures: attr.InvariantPredicateSignatures})
}
