package tokens

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/nop"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

var _ txtypes.Module = (*NopModule)(nil)

type NopModule struct {
	state         *state.State
	hashAlgorithm crypto.Hash
	execPredicate predicates.PredicateRunner
	pdr           types.PartitionDescriptionRecord
}

func NewNopModule(pdr types.PartitionDescriptionRecord, options *Options) *NopModule {
	return &NopModule{
		state:         options.state,
		hashAlgorithm: options.hashAlgorithm,
		execPredicate: predicates.NewPredicateRunner(options.exec),
		pdr:           pdr,
	}
}

func (m *NopModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		nop.TransactionTypeNOP: txtypes.NewTxHandler[nop.Attributes, nop.AuthProof](m.validateNopTx, m.executeNopTx),
	}
}

func (m *NopModule) validateNopTx(tx *types.TransactionOrder, attr *nop.Attributes, _ *nop.AuthProof, _ txtypes.ExecutionContext) error {
	unitID := tx.GetUnitID()
	unit, err := m.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("nop transaction: get unit error: %w", err)
	}
	if err := m.verifyCounter(unit.Data(), attr); err != nil {
		return fmt.Errorf("nop transaction: %w", err)
	}
	return nil
}

func (m *NopModule) executeNopTx(tx *types.TransactionOrder, _ *nop.Attributes, _ *nop.AuthProof, _ txtypes.ExecutionContext) (*types.ServerMetadata, error) {
	unitID := tx.GetUnitID()
	action := state.UpdateUnitData(unitID, m.incrementCounterFn())
	if err := m.state.Apply(action); err != nil {
		return nil, fmt.Errorf("nop transaction: failed to update state: %w", err)
	}
	return &types.ServerMetadata{TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (m *NopModule) verifyCounter(unitData types.UnitData, attr *nop.Attributes) error {
	if unitData == nil {
		if attr.Counter != nil {
			return errors.New("the transaction counter must be nil for dummy unit data")
		}
		return nil
	}

	switch data := unitData.(type) {
	case *tokens.FungibleTokenData:
		if attr.Counter == nil || *attr.Counter != data.Counter {
			return errors.New("the transaction counter is not equal to the unit counter for FT data")
		}
	case *tokens.NonFungibleTokenData:
		if attr.Counter == nil || *attr.Counter != data.Counter {
			return errors.New("the transaction counter is not equal to the unit counter for NFT data")
		}
	case *fc.FeeCreditRecord:
		if attr.Counter == nil || *attr.Counter != data.Counter {
			return errors.New("the transaction counter is not equal to the unit counter for FCR")
		}
	case *tokens.FungibleTokenTypeData:
		if attr.Counter != nil {
			return errors.New("the transaction counter must be nil for FT type")
		}
	case *tokens.NonFungibleTokenTypeData:
		if attr.Counter != nil {
			return errors.New("the transaction counter must be nil for NFT type")
		}
	default:
		return errors.New("invalid unit data type")
	}
	return nil
}

func (m *NopModule) incrementCounterFn() func(data types.UnitData) (types.UnitData, error) {
	return func(data types.UnitData) (types.UnitData, error) {
		if data == nil {
			return nil, nil // do nothing if dummy unit
		}
		switch d := data.(type) {
		case *tokens.FungibleTokenData:
			d.Counter += 1
			return d, nil
		case *tokens.NonFungibleTokenData:
			d.Counter += 1
			return d, nil
		case *fc.FeeCreditRecord:
			d.Counter += 1
			return d, nil
		case *tokens.FungibleTokenTypeData:
			return d, nil // do nothing, FT type does not have counter
		case *tokens.NonFungibleTokenTypeData:
			return d, nil // do nothing, NFT type does not have counter
		default:
			return nil, fmt.Errorf("invalid unit data type")
		}
	}
}
