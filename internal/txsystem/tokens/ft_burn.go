package tokens

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/holiman/uint256"
)

func handleBurnFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*burnFungibleTokenWrapper] {
	return func(tx *burnFungibleTokenWrapper, currentBlockNr uint64) error {
		logger.Debug("Processing Burn Fungible Token tx: %v", tx)
		if err := validateBurnFungibleToken(tx, options.state); err != nil {
			return fmt.Errorf("invalid burn fungible token transaction: %w", err)
		}
		fee := options.feeCalculator()
		tx.transaction.ServerMetadata = &txsystem.ServerMetadata{Fee: fee}
		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()

		unitID := tx.UnitID()
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, tx.Hash(options.hashAlgorithm)),
			rma.DeleteItem(unitID),
		)
	}
}

func validateBurnFungibleToken(tx *burnFungibleTokenWrapper, state *rma.Tree) error {
	d, err := getFungibleTokenData(tx.UnitID(), state)
	if err != nil {
		return err
	}
	tokenTypeID := d.tokenType.Bytes32()
	if !bytes.Equal(tokenTypeID[:], tx.attributes.Type) {
		return errors.Errorf("type of token to burn does not matches the actual type of the token: expected %X, got %X", tokenTypeID, tx.attributes.Type)
	}
	if tx.attributes.Value != d.value {
		return errors.Errorf("invalid token value: expected %v, got %v", d.value, tx.attributes.Value)
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
