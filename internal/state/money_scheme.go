package state

import (
	"crypto"
	"fmt"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state/tree"

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
		Owner tree.Predicate
	}

	BillData struct {
		V        uint64 // The monetary value of this bill
		T        uint64 // The round number of the last transaction with the bill
		Backlink []byte // Backlink (256-bit hash)
	}

	BillSummary struct {
		v uint64 // The uint64 value of summary
	}

	RevertibleState interface {
		AddItem(id *uint256.Int, owner tree.Predicate, data tree.Data, stateHash []byte) error
		DeleteItem(id *uint256.Int) error
		SetOwner(id *uint256.Int, owner tree.Predicate, stateHash []byte) error
		UpdateData(id *uint256.Int, f UpdateFunction, stateHash []byte) error
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

	defaultTree, err := tree.New(msState.hashAlgorithm)
	if err != nil {
		return nil, err
	}
	options := MoneySchemeOptions{
		unitsTree:       defaultTree,
		revertibleState: NewRevertible(defaultTree),
	}
	for _, o := range customOpts {
		o(&options)
	}

	msState.tree = options.unitsTree
	msState.revertibleState = options.revertibleState

	err = msState.tree.Set(initialBill.ID, initialBill.Owner, &BillData{
		V:        initialBill.Value,
		T:        0,
		Backlink: nil,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not set initial bill")
	}

	// TODO Not sure what the DC money owner predicate should be: https://guardtime.atlassian.net/browse/AB-93
	err = msState.tree.Set(dustCollectorMoneySupplyID, tree.Predicate{}, &BillData{
		V:        dcMoneyAmount,
		T:        0,
		Backlink: nil,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not set DC monet supply")
	}
	return msState, nil
}

func (m *moneySchemeState) Process(gtx GenericTransaction) error {
	// TODO transaction specific validity functions: https://guardtime.atlassian.net/browse/AB-92
	// TODO timeouts and backlinks: https://guardtime.atlassian.net/browse/AB-92
	// TODO add swap: https://guardtime.atlassian.net/browse/AB-46
	// TODO add transferDC: https://guardtime.atlassian.net/browse/AB-93
	switch tx := gtx.(type) {
	case Transfer:
		log.Debug("Processing transfer %v", tx)
		return m.revertibleState.SetOwner(tx.UnitId(), tx.NewBearer(), tx.Hash(m.hashAlgorithm))
	case Split:
		log.Debug("Processing split %v", tx)
		err := m.revertibleState.UpdateData(tx.UnitId(), func(data tree.Data) (newData tree.Data) {
			bd, ok := data.(*BillData)
			if !ok {
				// No change in case of incorrect data type.
				return data
			}
			return &BillData{
				V:        bd.V - tx.Amount(),
				T:        0,
				Backlink: nil,
			}
		}, tx.Hash(m.hashAlgorithm))
		if err != nil {
			return errors.Wrap(err, "could not update data")
		}

		newItemId := PrndSh(tx.UnitId(), tx.HashPrndSh(m.hashAlgorithm))
		err = m.revertibleState.AddItem(newItemId, tx.TargetBearer(), &BillData{
			V:        tx.Amount(),
			T:        0,
			Backlink: nil,
		}, nil)
		if err != nil {
			return errors.Wrapf(err, "could not add item")
		}

	default:
		return errors.New(fmt.Sprintf("Unknown type %T", gtx))
	}
	return nil
}

// GetRootHash starts root hash value computation and returns it.
func (m *moneySchemeState) GetRootHash() []byte {
	// TODO maybe it's better to delegate this though the reversible state
	return m.tree.GetRootHash()
}

// TotalValue starts tree calculation and returns the root node monetary value.
// It must remain constant during the lifetime of the state.
func (m *moneySchemeState) TotalValue() (uint64, error) {
	// TODO maybe it's better to delegate this though the reversible state
	sum := m.tree.GetSummaryValue()
	bs, ok := sum.(*BillSummary)
	if !ok {
		return 0, errors.New("summary was not *BillSummary")
	}
	return bs.v, nil
}

func (b *BillSummary) AddToHasher(hasher hash.Hash) {
	//TODO implement me
	panic("implement me")
}

func (b *BillSummary) Concatenate(left, right tree.SummaryValue) tree.SummaryValue {
	//TODO implement me
	panic("implement me")
}

func (b *BillData) AddToHasher(hasher hash.Hash) {
	//TODO implement me
	panic("implement me")
}

func (b *BillData) Value() tree.SummaryValue {
	//TODO implement me
	panic("implement me")
}
