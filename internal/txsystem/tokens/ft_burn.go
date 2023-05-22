package tokens

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleBurnFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[BurnFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *BurnFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Burn Fungible Token tx: %v", tx)
		if err := validateBurnFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid burn fungible token transaction: %w", err)
		}
		fee := options.feeCalculator()

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, tx.Hash(options.hashAlgorithm)),
			rma.DeleteItem(unitID),
		); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee}, nil

	}
}

func validateBurnFungibleToken(tx *types.TransactionOrder, attr *BurnFungibleTokenAttributes, state *rma.Tree) error {
	bearer, d, err := getFungibleTokenData(util.BytesToUint256(tx.UnitID()), state)
	if err != nil {
		return err
	}
	tokenTypeID := d.tokenType.Bytes32()
	if !bytes.Equal(tokenTypeID[:], attr.Type) {
		return fmt.Errorf("type of token to burn does not matches the actual type of the token: expected %X, got %X", tokenTypeID, attr.Type)
	}
	if attr.Value != d.value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.value, attr.Value)
	}
	if !bytes.Equal(d.backlink, attr.Backlink) {
		return fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
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
