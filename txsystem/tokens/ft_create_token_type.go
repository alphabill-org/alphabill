package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
)

func (m *FungibleTokensModule) handleCreateFungibleTokenTypeTx() txsystem.GenericExecuteFunc[tokens.CreateFungibleTokenTypeAttributes] {
	return func(tx *types.TransactionOrder, attr *tokens.CreateFungibleTokenTypeAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		if err := m.validateCreateFungibleTokenType(tx, attr); err != nil {
			return nil, fmt.Errorf("invalid create fungible token type tx: %w", err)
		}
		fee := m.feeCalculator()
		unitID := tx.UnitID()

		// update state
		if err := m.state.Apply(
			state.AddUnit(unitID, templates.AlwaysTrueBytes(), tokens.NewFungibleTokenTypeData(attr)),
		); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateCreateFungibleTokenType(tx *types.TransactionOrder, attr *tokens.CreateFungibleTokenTypeAttributes) error {
	unitID := tx.UnitID()
	if !unitID.HasType(tokens.FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	if attr.ParentTypeID != nil && !attr.ParentTypeID.HasType(tokens.FungibleTokenTypeUnitType) {
		return fmt.Errorf(ErrStrInvalidParentTypeID)
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

	u, err := m.state.GetUnit(unitID, false)
	if u != nil {
		return fmt.Errorf("unit %v exists", unitID)
	}
	if !errors.Is(err, avl.ErrNotFound) {
		return err
	}

	if attr.ParentTypeID != nil {
		parentData, err := getUnitData[*tokens.FungibleTokenTypeData](m.state.GetUnit, attr.ParentTypeID)
		if err != nil {
			return err
		}
		if decimalPlaces != parentData.DecimalPlaces {
			return fmt.Errorf("invalid decimal places. allowed %v, got %v", parentData.DecimalPlaces, decimalPlaces)
		}
	}

	err = runChainedPredicates[*tokens.FungibleTokenTypeData](
		tx,
		attr.ParentTypeID,
		attr.SubTypeCreationPredicateSignatures,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.SubTypeCreationPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("SubTypeCreationPredicate: %w", err)
	}
	return nil
}
