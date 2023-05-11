package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleTransferNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*transferNonFungibleTokenWrapper] {
	return func(tx *transferNonFungibleTokenWrapper, currentBlockNr uint64) error {
		logger.Debug("Processing Transfer Non-Fungible Token tx: %v", tx)
		if err := validateTransferNonFungibleToken(tx, options.state); err != nil {
			return fmt.Errorf("invalid transfer none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})

		// calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.SetOwner(tx.UnitID(), tx.attributes.NewBearer, h),
			rma.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
				d, ok := data.(*nonFungibleTokenData)
				if !ok {
					return data
				}
				d.t = currentBlockNr
				d.backlink = tx.Hash(options.hashAlgorithm)
				return data
			}, h))
	}
}

func validateTransferNonFungibleToken(tx *transferNonFungibleTokenWrapper, state *rma.Tree) error {
	unitID := tx.UnitID()
	u, err := state.GetUnit(unitID)
	if err != nil {
		return err
	}
	data, ok := u.Data.(*nonFungibleTokenData)
	if !ok {
		return fmt.Errorf("validate nft transfer: unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, tx.attributes.Backlink) {
		return errors.New("validate nft transfer: invalid backlink")
	}
	tokenTypeID := util.Uint256ToBytes(data.nftType)
	if !bytes.Equal(tx.NFTTypeID(), tokenTypeID) {
		return fmt.Errorf("invalid type identifier: expected '%X', got '%X'", tokenTypeID, tx.NFTTypeID())
	}

	// signature given in the transaction request satisfies the predicate obtained by concatenating all the token
	// invariant clauses along the type inheritance chain.
	predicates, err := getChainedPredicates(
		state,
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return verifyOwnership(Predicate(u.Bearer), predicates, tx)
}
