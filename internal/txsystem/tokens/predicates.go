package tokens

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/predicates"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	TokenOwnershipProver interface {
		types.SigBytesProvider
		OwnerProof() []byte
		InvariantPredicateSignatures() [][]byte
	}

	TokenSubtypeCreationProver interface {
		types.SigBytesProvider
		GetSubTypeCreationPredicateSignatures() [][]byte
	}

	TokenCreationProver interface {
		types.SigBytesProvider
		GetTokenCreationPredicateSignatures() [][]byte
	}

	NFTDataUpdateProver interface {
		types.SigBytesProvider
		GetDataUpdateSignatures() [][]byte
	}
)

func verifyPredicates(pbs []predicates.PredicateBytes, signatures [][]byte, sigData []byte) error {
	if len(pbs) != 0 {
		if len(pbs) != len(signatures) {
			return fmt.Errorf("number of signatures (%v) not equal to number of parent predicates (%v)", len(signatures), len(pbs))
		}
		for i := 0; i < len(pbs); i++ {
			err := predicates.RunPredicate(pbs[i], signatures[i], sigData)
			if err != nil {
				return fmt.Errorf("invalid predicate: %w [signature=0x%x predicate=0x%x sigData=0x%x]",
					err, signatures[i], pbs[i], sigData)
			}
		}
	}
	return nil
}

func getChainedPredicates[T state.UnitData](hashAlgorithm crypto.Hash, s *state.State, unitID types.UnitID, predicateFn func(d T) []byte, parentIDFn func(d T) types.UnitID) ([]predicates.PredicateBytes, error) {
	predicates := make([]predicates.PredicateBytes, 0)
	var parentID = unitID
	for {
		if parentID == nil {
			// type has no parent.
			break
		}
		_, parentData, err := getUnit[T](s, parentID)
		if err != nil {
			return nil, err
		}

		predicate := predicateFn(parentData)
		predicates = append(predicates, predicate)
		parentID = parentIDFn(parentData)
	}
	return predicates, nil
}

func getUnit[T state.UnitData](s *state.State, unitID types.UnitID) (*state.Unit, T, error) {
	u, err := s.GetUnit(unitID, false)
	if err != nil {
		return nil, *new(T), err
	}
	d, ok := u.Data().(T)
	if !ok {
		return nil, *new(T), fmt.Errorf("unit %v data is not of type %T", unitID, *new(T))
	}
	return u, d, nil
}

func verifyOwnership(bearer predicates.PredicateBytes, invariants []predicates.PredicateBytes, prover TokenOwnershipProver) error {
	predicates := append([]predicates.PredicateBytes{bearer}, invariants...)
	proofs := append([][]byte{prover.OwnerProof()}, prover.InvariantPredicateSignatures()...)
	sigBytes, err := prover.SigBytes()
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, proofs, sigBytes)
}
