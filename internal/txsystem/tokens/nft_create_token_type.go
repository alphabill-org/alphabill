package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/holiman/uint256"
)

func handleCreateNoneFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*createNonFungibleTokenTypeWrapper] {
	return func(tx *createNonFungibleTokenTypeWrapper, _ uint64) error {
		logger.Debug("Processing Create Non-Fungible Token Type tx: %v", tx.transaction.ToLogString(logger))
		if err := validate(tx, options.state); err != nil {
			return fmt.Errorf("invalid create none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		// calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.AddItem(tx.UnitID(), script.PredicateAlwaysTrue(), newNonFungibleTokenTypeData(tx), h))
	}
}

func validate(tx *createNonFungibleTokenTypeWrapper, state *rma.Tree) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return fmt.Errorf("create nft type: %s", ErrStrUnitIDIsZero)
	}
	if len(tx.Symbol()) > maxSymbolLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidSymbolLength)
	}
	if len(tx.Name()) > maxNameLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidNameLength)
	}
	if len(tx.Icon().GetType()) > maxIconTypeLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidIconTypeLength)
	}
	if len(tx.Icon().GetData()) > maxIconDataLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidIconDataLength)
	}

	u, err := state.GetUnit(unitID)
	if u != nil {
		return fmt.Errorf("create nft type: unit %v exists", unitID)
	}
	if !errors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	// signature satisfies the predicate obtained by concatenating all the
	// sub-type creation clauses along the type inheritance chain.
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		state,
		tx.parentTypeIdInt(),
		func(d *nonFungibleTokenTypeData) []byte {
			return d.subTypeCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, tx.SubTypeCreationPredicateSignatures(), tx.SigBytes())
}
