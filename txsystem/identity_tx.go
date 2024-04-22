package txsystem

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
)

var _ Module = (*IdentityModule)(nil)

const TxIdentity = "identity"

type IdentityModule struct {
	state UnitState
	pr    predicates.PredicateRunner
}

type IdentityAttributes struct {
	_     struct{} `cbor:",toarray"`
	Nonce []byte
}

func NewIdentityModule(state UnitState) *IdentityModule {
	engines, err := predicates.Dispatcher(templates.New())
	if err != nil {
		panic(fmt.Errorf("creating predicate executor: %w", err))
	}
	pr := predicates.NewPredicateRunner(engines.Execute, state)
	return &IdentityModule{state: state, pr: pr}
}

func (i *IdentityModule) TxExecutors() map[string]ExecuteFunc {
	return map[string]ExecuteFunc{
		TxIdentity: i.handleIdentityTx().ExecuteFunc(),
	}
}

func (i *IdentityModule) handleIdentityTx() GenericExecuteFunc[IdentityAttributes] {
	return func(tx *types.TransactionOrder, attr *IdentityAttributes, exeCtx *TxExecutionContext) (*types.ServerMetadata, error) {
		if err := i.validateIdentityTx(tx, exeCtx); err != nil {
			return nil, fmt.Errorf("invalid identity tx: %w", err)
		}

		return &types.ServerMetadata{ActualFee: 1, TargetUnits: []types.UnitID{tx.UnitID()}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (i *IdentityModule) validateIdentityTx(tx *types.TransactionOrder, exeCtx *TxExecutionContext) (err error) {
	if tx.Payload == nil {
		return fmt.Errorf("missing payload")
	}

	if tx.Payload.Type != TxIdentity {
		return fmt.Errorf("invalid tx type: %s", tx.Payload.Type)
	}

	if !exeCtx.StateLockReleased {
		unitID := tx.UnitID()
		var u *state.Unit
		u, err = i.state.GetUnit(unitID, false)
		if err != nil {
			return fmt.Errorf("identity tx: unable to fetch the unit: %w", err)
		}

		if err = i.verifyUnitOwnerProof(tx, u.Bearer()); err != nil {
			return fmt.Errorf("identity tx: %w", err)
		}

		_, err = LockUnitState(tx, i.pr, i.state)
		if err != nil {
			return fmt.Errorf("identity tx, failed to lock state: %w", err)
		}
	}
	return nil
}

func (i *IdentityModule) verifyUnitOwnerProof(tx *types.TransactionOrder, bearer types.PredicateBytes) error {
	if err := i.pr(bearer, tx.OwnerProof, tx); err != nil {
		return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x]",
			err, tx.OwnerProof, bearer)
	}

	return nil
}
