package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleCreateNoneFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[CreateNonFungibleTokenTypeAttributes] {
	return func(tx *types.TransactionOrder, attr *CreateNonFungibleTokenTypeAttributes, currentBlockNumber uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Create Non-Fungible Token Type tx: %v", tx)
		if err := validate(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid create none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		// disable fee handling if fee is calculated to 0 (used to temporarily disable fee handling, can be removed after all wallets are updated)
		var fcFunc rma.Action
		if options.feeCalculator() == 0 {
			fcFunc = func(tree *rma.Tree) error {
				return nil
			}
		} else {
			fcrID := tx.GetClientFeeCreditRecordID()
			fcFunc = fc.DecrCredit(util.BytesToUint256(fcrID), fee, h)
		}

		if err := options.state.AtomicUpdate(
			fcFunc,
			rma.AddItem(util.BytesToUint256(tx.UnitID()), script.PredicateAlwaysTrue(), newNonFungibleTokenTypeData(attr), h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validate(tx *types.TransactionOrder, attr *CreateNonFungibleTokenTypeAttributes, state *rma.Tree) error {
	unitID := util.BytesToUint256(tx.UnitID())
	if unitID.IsZero() {
		return fmt.Errorf("create nft type: %s", ErrStrUnitIDIsZero)
	}
	if len(attr.Symbol) > maxSymbolLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidSymbolLength)
	}
	if len(attr.Name) > maxNameLength {
		return fmt.Errorf("create nft type: %s", ErrStrInvalidNameLength)
	}
	if attr.Icon != nil {
		if len(attr.Icon.Type) > maxIconTypeLength {
			return fmt.Errorf("create nft type: %s", ErrStrInvalidIconTypeLength)
		}
		if len(attr.Icon.Data) > maxIconDataLength {
			return fmt.Errorf("create nft type: %s", ErrStrInvalidIconDataLength)
		}
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
		util.BytesToUint256(attr.ParentTypeID),
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
	sigBytes, err := tx.PayloadBytes()
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, attr.SubTypeCreationPredicateSignatures, sigBytes)
}
