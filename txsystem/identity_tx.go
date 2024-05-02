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
	state *state.State
	pr    predicates.PredicateRunner
}

type IdentityAttributes struct {
	_     struct{} `cbor:",toarray"`
	Nonce []byte
}

func NewIdentityModule(state *state.State) (*IdentityModule, error) {
	if state == nil {
		return nil, fmt.Errorf("state is nil")
	}
	engines, err := predicates.Dispatcher(templates.New())
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}
	pr := predicates.NewPredicateRunner(engines.Execute, state)
	return &IdentityModule{state: state, pr: pr}, nil
}

func (i *IdentityModule) TxHandlers() map[string]TxExecutor {
	return map[string]TxExecutor{
		TxIdentity: NewTxHandler[IdentityAttributes](i.validateIdentityTx, i.executeIdentityTx),
	}
}

func (i *IdentityModule) executeIdentityTx(tx *types.TransactionOrder, _ *IdentityAttributes, _ *TxExecutionContext) (*types.ServerMetadata, error) {
	// this is basically nop tx
	return &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful}, nil
}

func (i *IdentityModule) validateIdentityTx(tx *types.TransactionOrder, _ *IdentityAttributes, _ *TxExecutionContext) (err error) {
	if tx == nil {
		return fmt.Errorf("tx is nil")
	}
	// identity must either lock or unlock a unit
	unitID := tx.UnitID()
	var u *state.Unit
	u, err = i.state.GetUnit(unitID, false)
	if err != nil {
		return fmt.Errorf("unable to fetch the unit: %w", err)
	}
	if u.IsStateLocked() && tx.StateUnlock == nil {
		return fmt.Errorf("missing unlock arguments")
	}
	// todo: why should lock be ignored, but not unlock?
	/*	if !u.IsStateLocked() && tx.Payload.StateLock == nil {
		return fmt.Errorf("missing lock predicate")
	}*/
	if err = i.verifyUnitOwnerProof(tx, u.Bearer()); err != nil {
		return fmt.Errorf("owner proof error: %w", err)
	}
	return nil
}

func (i *IdentityModule) verifyUnitOwnerProof(tx *types.TransactionOrder, bearer types.PredicateBytes) error {
	err := i.pr(bearer, tx.OwnerProof, tx)
	if err != nil {
		return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x]",
			err, tx.OwnerProof, bearer)
	}
	return nil
}
