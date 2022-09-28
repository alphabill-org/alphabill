package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/holiman/uint256"
)

type baseTxExecutor[T rma.UnitData] struct {
	zeroValue     T
	state         *rma.Tree
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

func (b *baseTxExecutor[T]) getChainedPredicate(unitID *uint256.Int, predicateFn func(d T) []byte, parentIDFn func(d T) *uint256.Int) ([]byte, error) {
	var predicate []byte
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
		predicate = append(predicateFn(parentData), predicate...)
		parentID = parentIDFn(parentData)
	}
	return predicate, nil
}
