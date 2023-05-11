package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/holiman/uint256"
)

func handleMintFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*mintFungibleTokenWrapper] {
	return func(tx *mintFungibleTokenWrapper, currentBlockNr uint64) error {
		logger.Debug("Processing Mint Fungible Token tx: %v", tx)
		if err := validateMintFungibleToken(tx, options.state); err != nil {
			return fmt.Errorf("invalid mint fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		// calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.AddItem(tx.UnitID(), tx.attributes.Bearer, newFungibleTokenData(tx, h, currentBlockNr), h),
		)
	}
}

func validateMintFungibleToken(tx *mintFungibleTokenWrapper, state *rma.Tree) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	u, err := state.GetUnit(unitID)
	if u != nil {
		return fmt.Errorf("unit with id %v already exists", unitID)
	}
	if !errors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	if tx.Value() == 0 {
		return errors.New("token must have value greater than zero")
	}

	// existence of the parent type is checked by the getChainedPredicates
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		state,
		tx.TypeIDInt(),
		func(d *fungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, tx.TokenCreationPredicateSignatures(), tx.SigBytes())
}
