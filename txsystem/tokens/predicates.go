package tokens

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

/*
runChainedPredicates recursively executes predicates returned by callback functions.
Typically, these are predicates of the token type chain.

Parameters:
  - "env": transaction execution environment/context;
  - "txo": transaction order against which predicates are run,
    passed on as argument to the exec callback;
  - "parentID": ID of the first unit in the chain, typically token's type ID;
  - "args": slice of arguments for chained predicates, ie when called for mint NFT tx the
    TokenMintingProofs field of the mint tx;
  - "exec": function which evaluates the predicate;
  - "iter": function which returns "parent ID" and predicate for given unit (the "chain iterator");
  - "getUnit": function which returns unit with given ID;
*/
func runChainedPredicates[T types.UnitData](
	env txtypes.ExecutionContext,
	txo *types.TransactionOrder,
	parentID types.UnitID,
	args [][]byte,
	exec predicates.PredicateRunner,
	iter func(d T) (types.UnitID, []byte),
	getUnit func(id types.UnitID, committed bool) (state.VersionedUnit, error),
) error {
	var predicate []byte
	idx := 0
	for ; parentID != nil; idx++ {
		if idx >= len(args) {
			return fmt.Errorf("got arguments for %d predicates but chain is longer, no argument for unit %q predicate", len(args), parentID)
		}
		parentData, err := getUnitData[T](getUnit, parentID)
		if err != nil {
			return fmt.Errorf("read [%d] unit ID %q data: %w", idx, parentID, err)
		}
		if parentID, predicate = iter(parentData); predicate == nil {
			return fmt.Errorf("unexpected nil predicate")
		}
		if err := exec(predicate, args[idx], txo, env); err != nil {
			return fmt.Errorf("executing predicate [%d] in the chain: %w", idx, err)
		}
	}
	if idx != len(args) {
		return fmt.Errorf("got arguments for %d predicates, executed %d predicates", len(args), idx)
	}
	return nil
}

func getUnitData[T types.UnitData](getUnit func(id types.UnitID, committed bool) (state.VersionedUnit, error), unitID types.UnitID) (T, error) {
	u, err := getUnit(unitID, false)
	if err != nil {
		return *new(T), err
	}
	d, ok := u.Data().(T)
	if !ok {
		return *new(T), fmt.Errorf("expected unit %v data to be %T got %T", unitID, *new(T), u.Data())
	}
	return d, nil
}
