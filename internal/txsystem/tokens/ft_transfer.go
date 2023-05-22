package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleTransferFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[TransferFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Transfer Fungible Token tx: %v", tx)
		if err := validateTransferFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid transfer fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.SetOwner(unitID, attr.NewBearer, h),
			rma.UpdateData(unitID,
				func(data rma.UnitData) (newData rma.UnitData) {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						return data
					}
					d.t = currentBlockNr
					d.backlink = tx.Hash(options.hashAlgorithm)
					return data
				}, h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateTransferFungibleToken(tx *types.TransactionOrder, attr *TransferFungibleTokenAttributes, state *rma.Tree) error {
	bearer, d, err := getFungibleTokenData(util.BytesToUint256(tx.UnitID()), state)
	if err != nil {
		return err
	}
	if d.value != attr.Value {
		return fmt.Errorf("invalid token value: expected %v, got %v", d.value, attr.Value)
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
