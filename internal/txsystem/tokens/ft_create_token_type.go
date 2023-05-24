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
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
)

var ErrStrUnitIDIsZero = "unit ID cannot be zero"

func handleCreateFungibleTokenTypeTx(options *Options) txsystem.GenericExecuteFunc[CreateFungibleTokenTypeAttributes] {
	return func(tx *types.TransactionOrder, attr *CreateFungibleTokenTypeAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Create Fungible Token Type tx: %v", tx)
		if err := validateCreateFungibleTokenType(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid create fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.AddItem(unitID, script.PredicateAlwaysTrue(), newFungibleTokenTypeData(attr), h),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateCreateFungibleTokenType(tx *types.TransactionOrder, attr *CreateFungibleTokenTypeAttributes, state *rma.Tree) error {
	unitID := util.BytesToUint256(tx.UnitID())
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	if len(attr.Symbol) > maxSymbolLength {
		return errors.New(ErrStrInvalidSymbolLength)
	}
	if len(attr.Name) > maxNameLength {
		return errors.New(ErrStrInvalidNameLength)
	}
	if attr.Icon != nil {
		if len(attr.Icon.Type) > maxIconTypeLength {
			return errors.New(ErrStrInvalidIconTypeLength)
		}
		if len(attr.Icon.Data) > maxIconDataLength {
			return errors.New(ErrStrInvalidIconDataLength)
		}
	}

	decimalPlaces := attr.DecimalPlaces
	if decimalPlaces > maxDecimalPlaces {
		return fmt.Errorf("invalid decimal places. maximum allowed value %v, got %v", maxDecimalPlaces, decimalPlaces)
	}

	u, err := state.GetUnit(unitID)
	if u != nil {
		return fmt.Errorf("unit %v exists", unitID)
	}
	if !errors.Is(err, rma.ErrUnitNotFound) {
		return err
	}

	parentUnitID := util.BytesToUint256(attr.ParentTypeID)
	if !parentUnitID.IsZero() {
		_, parentData, err := getUnit[*fungibleTokenTypeData](state, parentUnitID)
		if err != nil {
			return err
		}
		if decimalPlaces != parentData.decimalPlaces {
			return fmt.Errorf("invalid decimal places. allowed %v, got %v", parentData.decimalPlaces, decimalPlaces)
		}
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		state,
		util.BytesToUint256(attr.ParentTypeID),
		func(d *fungibleTokenTypeData) []byte {
			return d.subTypeCreationPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}

	sigBytes, err := getCreateFungibleTokenTypeSignedData(tx, attr)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, attr.SubTypeCreationPredicateSignatures, sigBytes)
}

func getCreateFungibleTokenTypeSignedData(tx *types.TransactionOrder, attr *CreateFungibleTokenTypeAttributes) ([]byte, error) {
	// TODO SubTypeCreationPredicateSignatures field can not be part of payload attributes.
	if len(attr.SubTypeCreationPredicateSignatures) > 0 {
		// exclude SubTypeCreationPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
		signatureAttr := &CreateFungibleTokenTypeAttributes{
			Symbol:                             attr.Symbol,
			Name:                               attr.Name,
			Icon:                               attr.Icon,
			ParentTypeID:                       attr.ParentTypeID,
			DecimalPlaces:                      attr.DecimalPlaces,
			SubTypeCreationPredicate:           attr.SubTypeCreationPredicate,
			TokenCreationPredicate:             attr.TokenCreationPredicate,
			InvariantPredicate:                 attr.InvariantPredicate,
			SubTypeCreationPredicateSignatures: nil,
		}
		attrBytes, err := cbor.Marshal(signatureAttr)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal attributes: %w", err)
		}
		payload := &types.Payload{
			SystemID:       tx.Payload.SystemID,
			Type:           tx.Payload.Type,
			UnitID:         tx.Payload.UnitID,
			Attributes:     attrBytes,
			ClientMetadata: tx.Payload.ClientMetadata,
		}
		return payload.Bytes()
	}
	return tx.PayloadBytes()
}
