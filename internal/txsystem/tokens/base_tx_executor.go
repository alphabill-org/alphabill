package tokens

import (
	"crypto"
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
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

func (b *baseTxExecutor[T]) getFungibleTokenData(unitID *uint256.Int) (*fungibleTokenData, error) {
	if unitID.IsZero() {
		return nil, errors.New(ErrStrUnitIDIsZero)
	}
	u, err := b.state.GetUnit(unitID)
	if err != nil {
		if goerrors.Is(err, rma.ErrUnitNotFound) {
			return nil, errors.Wrapf(err, "unit %v does not exist", unitID)
		}
		return nil, err
	}
	d, ok := u.Data.(*fungibleTokenData)
	if !ok {
		return nil, errors.Errorf("unit %v is not fungible token data", unitID)
	}
	return d, nil
}
