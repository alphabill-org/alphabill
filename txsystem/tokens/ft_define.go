package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func (m *FungibleTokensModule) executeDefineFT(tx *types.TransactionOrder, attr *tokens.DefineFungibleTokenAttributes, _ *tokens.DefineFungibleTokenAuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()

	if err := m.state.Apply(
		state.AddUnit(unitID, tokens.NewFungibleTokenTypeData(attr)),
	); err != nil {
		return nil, err
	}

	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *FungibleTokensModule) validateDefineFT(tx *types.TransactionOrder, attr *tokens.DefineFungibleTokenAttributes, authProof *tokens.DefineFungibleTokenAuthProof, exeCtx txtypes.ExecutionContext) error {
	unitID := tx.GetUnitID()
	if err := unitID.TypeMustBe(tokens.FungibleTokenTypeUnitType, &m.pdr); err != nil {
		return fmt.Errorf("invalid unit ID: %w", err)
	}
	if attr.ParentTypeID != nil {
		if err := attr.ParentTypeID.TypeMustBe(tokens.FungibleTokenTypeUnitType, &m.pdr); err != nil {
			return fmt.Errorf("invalid parent type: %w", err)
		}
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
		exeCtx,
		tx.AuthProofSigBytes,
		attr.ParentTypeID,
		authProof.SubTypeCreationProofs,
		m.execPredicate,
		func(d *tokens.FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeID, d.SubTypeCreationPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return fmt.Errorf("SubTypeCreationPredicate: %w", err)
	}
	return nil
}
