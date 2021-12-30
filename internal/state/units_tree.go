package state

import "github.com/holiman/uint256"

type (
	unitsTreeImpl struct {
	}
)

func NewUnitsTree() *unitsTreeImpl {
	return &unitsTreeImpl{}
}

func (u unitsTreeImpl) Delete(id *uint256.Int) error {
	//TODO implement me
	panic("implement me")
}

func (u unitsTreeImpl) Get(id *uint256.Int) (owner Predicate, data Data, err error) {
	//TODO implement me
	panic("implement me")
}

func (u unitsTreeImpl) Set(id *uint256.Int, owner Predicate, data Data) error {
	//TODO implement me
	panic("implement me")
}

func (u unitsTreeImpl) SetOwner(id *uint256.Int, owner Predicate) error {
	//TODO implement me
	panic("implement me")
}

func (u unitsTreeImpl) SetData(id *uint256.Int, data Data) error {
	//TODO implement me
	panic("implement me")
}

func (u unitsTreeImpl) Exists(id *uint256.Int) (bool, error) {
	//TODO implement me
	panic("implement me")
}

func (u unitsTreeImpl) GetRootHash() []byte {
	//TODO implement me
	panic("implement me")
}

func (u unitsTreeImpl) GetSummaryValue() SummaryValue {
	//TODO implement me
	panic("implement me")
}
