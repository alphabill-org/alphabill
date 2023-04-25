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

func handleTransferFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*transferFungibleTokenWrapper] {
	return func(tx *transferFungibleTokenWrapper, currentBlockNr uint64) error {
		logger.Debug("Processing Transfer Fungible Token tx: %v", tx)
		if err := validateTransferFungibleToken(tx, options.state); err != nil {
			return fmt.Errorf("invalid transfer fungible token tx: %w", err)
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
			rma.UpdateData(tx.UnitID(),
				func(data rma.UnitData) (newData rma.UnitData) {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						return data
					}
					d.t = currentBlockNr
					d.backlink = tx.Hash(options.hashAlgorithm)
					return data
				}, h))
	}
}

func validateTransferFungibleToken(tx *transferFungibleTokenWrapper, state *rma.Tree) error {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), state)
	if err != nil {
		return err
	}
	if d.value != tx.attributes.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.value, tx.attributes.Value)
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

func getFungibleTokenData(unitID *uint256.Int, state *rma.Tree) (Predicate, *fungibleTokenData, error) {
	if unitID.IsZero() {
		return nil, nil, errors.New(ErrStrUnitIDIsZero)
	}
	u, err := state.GetUnit(unitID)
	if err != nil {
		if errors.Is(err, rma.ErrUnitNotFound) {
			return nil, nil, fmt.Errorf("unit %v does not exist: %w", unitID, err)
		}
		return nil, nil, err
	}
	d, ok := u.Data.(*fungibleTokenData)
	if !ok {
		return nil, nil, fmt.Errorf("unit %v is not fungible token data", unitID)
	}
	return Predicate(u.Bearer), d, nil
}
