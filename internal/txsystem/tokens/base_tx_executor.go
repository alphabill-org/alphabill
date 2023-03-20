package tokens

import (
	"crypto"
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/holiman/uint256"
)

type baseTxExecutor[T rma.UnitData] struct {
	zeroValue     T
	state         TokenState
	hashAlgorithm crypto.Hash
}

func (b *baseTxExecutor[T]) getUnit(unitID *uint256.Int) (*rma.Unit, T, error) {
	u, err := b.state.GetUnit(unitID)
	if err != nil {
		return nil, b.zeroValue, err
	}
	d, ok := u.Data.(T)
	if !ok {
		return nil, b.zeroValue, errors.Errorf("unit %v data is not of type %T", unitID, b.zeroValue)
	}
	return u, d, nil
}

func (b *baseTxExecutor[T]) getChainedPredicates(unitID *uint256.Int, predicateFn func(d T) []byte, parentIDFn func(d T) *uint256.Int) ([]Predicate, error) {
	predicates := make([]Predicate, 0)
	var parentID = unitID
	for {
		if parentID.IsZero() {
			// type has no parent.
			break
		}
		_, parentData, err := b.getUnit(parentID)
		if err != nil {
			return nil, err
		}

		predicate := predicateFn(parentData)
		predicates = append(predicates, predicate)
		parentID = parentIDFn(parentData)
	}
	return predicates, nil
}

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

type TokenOwnershipProver interface {
	OwnerProof() []byte
	InvariantPredicateSignatures() [][]byte
	SigBytes() []byte
}

func verifyOwnership(bearer Predicate, invPredicates []Predicate, prover TokenOwnershipProver) error {
	predicates := append([]Predicate{bearer}, invPredicates...)
	proofs := append([][]byte{prover.OwnerProof()}, prover.InvariantPredicateSignatures()...)
	return verifyPredicates(predicates, proofs, prover.SigBytes())
}

func (b *baseTxExecutor[T]) getFungibleTokenData(unitID *uint256.Int) (Predicate, *fungibleTokenData, error) {
	if unitID.IsZero() {
		return nil, nil, errors.New(ErrStrUnitIDIsZero)
	}
	u, err := b.state.GetUnit(unitID)
	if err != nil {
		if goerrors.Is(err, rma.ErrUnitNotFound) {
			return nil, nil, errors.Wrapf(err, "unit %v does not exist", unitID)
		}
		return nil, nil, err
	}
	d, ok := u.Data.(*fungibleTokenData)
	if !ok {
		return nil, nil, errors.Errorf("unit %v is not fungible token data", unitID)
	}
	return Predicate(u.Bearer), d, nil
}
