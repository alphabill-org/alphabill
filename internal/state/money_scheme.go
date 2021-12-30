package state

import (
	"crypto"
	"fmt"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"github.com/holiman/uint256"
)

type (
	moneySchemeState struct {
		revertibleState RevertibleState
		tree            UnitsTree
		hashAlgorithm   crypto.Hash // hash function algorithm
	}

	InitialBill struct {
		ID    *uint256.Int
		Value uint64
		Owner Predicate
	}

	BillData struct {
		Value    uint64 // The monetary value of this bill
		T        uint64 // The round number of the last transaction with the bill
		Backlink []byte // Backlink (256-bit hash)
	}

	RevertibleState interface {
		AddItem(id *uint256.Int, owner Predicate, data Data) error
		DeleteItem(id *uint256.Int) error
		SetOwner(id *uint256.Int, owner Predicate) error
		UpdateData(id *uint256.Int, f UpdateFunction) error
		Revert() error
		Commit()
	}
)

// The ID of the dust collector money supply
var dustCollectorMoneySupplyID = uint256.NewInt(0)

func NewMoneySchemeState(initialBill *InitialBill, dcMoneyAmount uint64, customOpts ...MoneySchemeOption) (*moneySchemeState, error) {
	msState := &moneySchemeState{
		hashAlgorithm: crypto.SHA256, // TODO add hash function argument
	}

	defaultTree := NewUnitsTree()
	options := MoneySchemeOptions{
		unitsTree:       defaultTree,
		revertibleState: NewRevertible(defaultTree),
	}
	for _, o := range customOpts {
		o(&options)
	}

	msState.tree = options.unitsTree
	msState.revertibleState = options.revertibleState

	err := msState.tree.Set(initialBill.ID, initialBill.Owner, BillData{
		Value:    initialBill.Value,
		T:        0,
		Backlink: nil,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not set initial bill")
	}

	// TODO Not sure what the DC money owner predicate should be
	err = msState.tree.Set(dustCollectorMoneySupplyID, Predicate{}, BillData{
		Value:    dcMoneyAmount,
		T:        0,
		Backlink: nil,
	})
	if err != nil {
		return nil, errors.Wrap(err, "could not set DC monet supply")
	}
	return msState, nil
}

func (m *moneySchemeState) Process(gtx GenericTransaction) error {
	switch tx := gtx.(type) {
	case Transfer:
		log.Debug("Processing transfer %v", tx)
		// TODO transaction specific validity function
		return m.revertibleState.SetOwner(tx.UnitId(), tx.NewBearer())
	case Split:
		log.Debug("Processing split %v", tx)
		// TODO transaction specific validity function
		err := m.revertibleState.UpdateData(tx.UnitId(), func(data Data) (newData Data) {
			bd, ok := data.(BillData)
			if !ok {
				// No change in case of incorrect data type.
				return data
			}
			return BillData{
				Value:    bd.Value - tx.Amount(),
				T:        0, // TODO timeout and backlink
				Backlink: nil,
			}
		})
		if err != nil {
			return errors.Wrap(err, "could not update data")
		}

		newItemId := PrndSh(tx.UnitId(), tx.HashPrndSh(m.hashAlgorithm))
		err = m.revertibleState.AddItem(newItemId, tx.TargetBearer(), BillData{
			Value:    tx.Amount(),
			T:        0, // TODO timeout and backlink
			Backlink: nil,
		})
		if err != nil {
			return errors.Wrapf(err, "could not add item")
		}

		// TODO ... other types
	default:
		return errors.New(fmt.Sprintf("Unknown type %T", gtx))
	}
	return nil
}

// GetRootHash starts root hash value computation and returns it.
func (m *moneySchemeState) GetRootHash() []byte {
	return m.tree.GetRootHash()
}

// TotalValue starts tree calculation and returns the root node monetary value.
// It must remain constant during the lifetime of the state.
func (m *moneySchemeState) TotalValue() (uint64, error) {
	sum := m.tree.GetSummaryValue()
	intVal, ok := sum.(uint64)
	if !ok {
		return 0, errors.New("summary was not uint64")
	}
	return intVal, nil
}
