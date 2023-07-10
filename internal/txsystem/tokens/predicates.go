package tokens

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/script"
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

func verifyPredicates(predicates []state.Predicate, signatures [][]byte, sigData []byte) error {
	if len(predicates) != 0 {
		if len(predicates) != len(signatures) {
			return fmt.Errorf("number of signatures (%v) not equal to number of parent predicates (%v)", len(signatures), len(predicates))
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

func getChainedPredicates[T state.UnitData](hashAlgorithm crypto.Hash, s *state.State, unitID types.UnitID, predicateFn func(d T) []byte, parentIDFn func(d T) types.UnitID) ([]state.Predicate, error) {
	predicates := make([]state.Predicate, 0)
	var parentID = unitID
	for {
		if parentID.IsZero(hashAlgorithm.Size()) {
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

func verifyOwnership(bearer state.Predicate, invariants []state.Predicate, prover TokenOwnershipProver) error {
	predicates := append([]state.Predicate{bearer}, invariants...)
	proofs := append([][]byte{prover.OwnerProof()}, prover.InvariantPredicateSignatures()...)
	sigBytes, err := prover.SigBytes()
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, proofs, sigBytes)
}
