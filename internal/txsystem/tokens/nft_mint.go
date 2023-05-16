package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleMintNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*mintNonFungibleTokenWrapper] {
	return func(tx *mintNonFungibleTokenWrapper, currentBlockNr uint64) error {
		logger.Debug("Processing Mint Non-Fungible Token tx: %v", tx.transaction.ToLogString(logger))
		if err := validateMintNonFungibleToken(tx, options.state); err != nil {
			return fmt.Errorf("invalid mint none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		// calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.AddItem(tx.UnitID(), tx.attributes.Bearer, newNonFungibleTokenData(tx, h, currentBlockNr), h))
	}
}

func validateMintNonFungibleToken(tx *mintNonFungibleTokenWrapper, state *rma.Tree) error {
	unitID := tx.wrapper.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	if len(tx.Name()) > maxNameLength {
		return errors.New(ErrStrInvalidNameLength)
	}
	uri := tx.URI()
	if uri != "" {
		if len(uri) > uriMaxSize {
			return fmt.Errorf("URI exceeds the maximum allowed size of %v KB", uriMaxSize)
		}
		if !util.IsValidURI(uri) {
			return fmt.Errorf("URI %s is invalid", uri)
		}
	}
	if len(tx.Data()) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	u, err := state.GetUnit(unitID)
	if u != nil {
		return fmt.Errorf("unit %v exists", unitID)
	}
	if !errors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	nftTypeID := tx.NFTTypeIDInt()
	if nftTypeID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}

	// the transaction request satisfies the predicate obtained by concatenating all the token creation clauses along
	// the type inheritance chain.
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		state,
		nftTypeID,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, tx.TokenCreationPredicateSignatures(), tx.SigBytes())
}
