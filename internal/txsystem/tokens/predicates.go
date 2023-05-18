package tokens

import (
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/holiman/uint256"
)

func verifyPredicates(predicates []Predicate, signatures [][]byte, sigData []byte) error {
	if len(predicates) != 0 {
		if len(predicates) != len(signatures) {
			return errors.Errorf("Number of signatures (%v) not equal to number of parent predicates (%v)", len(signatures), len(predicates))
		}
		for i := 0; i < len(predicates); i++ {
			err := script.RunScript(signatures[i], predicates[i], sigData)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func getChainedPredicates[T rma.UnitData](state *rma.Tree, unitID *uint256.Int, predicateFn func(d T) []byte, parentIDFn func(d T) *uint256.Int) ([]Predicate, error) {
	predicates := make([]Predicate, 0)
	var parentID = unitID
	for {
		if parentID.IsZero() {
			// type has no parent.
			break
		}
		_, parentData, err := getUnit[T](state, parentID)
		if err != nil {
			return nil, err
		}

		predicate := predicateFn(parentData)
		predicates = append(predicates, predicate)
		parentID = parentIDFn(parentData)
	}
	return predicates, nil
}

func getUnit[T rma.UnitData](state *rma.Tree, unitID *uint256.Int) (*rma.Unit, T, error) {
	u, err := state.GetUnit(unitID)
	if err != nil {
		return nil, *new(T), err
	}
	d, ok := u.Data.(T)
	if !ok {
		return nil, *new(T), errors.Errorf("unit %v data is not of type %T", unitID, *new(T))
	}
	return u, d, nil
}

type TokenOwnershipProver interface {
	OwnerProof() []byte
	InvariantPredicateSignatures() [][]byte
	SigBytes() []byte
}

func verifyOwnership(bearer Predicate, invariants []Predicate, prover TokenOwnershipProver) error {
	predicates := append([]Predicate{bearer}, invariants...)
	proofs := append([][]byte{prover.OwnerProof()}, prover.InvariantPredicateSignatures()...)
	return verifyPredicates(predicates, proofs, prover.SigBytes())
}
