package txsystem

import (
	"crypto"
	"fmt"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"github.com/holiman/uint256"
)

type (
	GenericTransaction interface {
		UnitId() *uint256.Int
		Timeout() uint64
		OwnerProof() []byte
		Hash(hashFunc crypto.Hash) []byte
	}

	Transfer interface {
		GenericTransaction
		NewBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	TransferDC interface {
		GenericTransaction
		Nonce() []byte
		TargetBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	Split interface {
		GenericTransaction
		Amount() uint64
		TargetBearer() []byte
		RemainingValue() uint64
		Backlink() []byte
		HashPrndSh(hashFunc crypto.Hash) []byte // Returns hash value for the PrndSh function
	}

	Swap interface {
		GenericTransaction
		OwnerCondition() []byte
		BillIdentifiers() []*uint256.Int
		DCTransfers() []TransferDC
		Proofs() [][]byte
		TargetValue() uint64
	}

	InitialBill struct {
		ID    *uint256.Int
		Value uint64
		Owner state.Predicate
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
		AddItem(id *uint256.Int, owner state.Predicate, data state.UnitData, stateHash []byte) error
		DeleteItem(id *uint256.Int) error
		SetOwner(id *uint256.Int, owner state.Predicate, stateHash []byte) error
		UpdateData(id *uint256.Int, f state.UpdateFunction, stateHash []byte) error
		Revert() error
		Commit()
		GetRootHash() []byte
		TotalValue() state.SummaryValue
	}

	moneySchemeState struct {
		revertibleState RevertibleState
		hashAlgorithm   crypto.Hash // hash function algorithm
	}
)

var log = logger.CreateForPackage()

// The ID of the dust collector money supply
var dustCollectorMoneySupplyID = uint256.NewInt(0)

func NewMoneySchemeState(hashAlgorithm crypto.Hash, initialBill *InitialBill, dcMoneyAmount uint64, customOpts ...MoneySchemeOption) (*moneySchemeState, error) {
	// TODO validate that initialBillID doesn't match with DC money ID. https://guardtime.atlassian.net/browse/AB-93
	defaultTree, err := state.NewRevertible(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	options := MoneySchemeOptions{
		revertibleState: defaultTree,
	}
	for _, o := range customOpts {
		o(&options)
	}

	msState := &moneySchemeState{
		hashAlgorithm:   hashAlgorithm,
		revertibleState: options.revertibleState,
	}

	err = msState.revertibleState.AddItem(initialBill.ID, initialBill.Owner, &BillData{
		V:        initialBill.Value,
		T:        0,
		Backlink: nil,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not set initial bill")
	}

	// TODO Not sure what the DC money owner predicate should be: https://guardtime.atlassian.net/browse/AB-93
	err = msState.revertibleState.AddItem(dustCollectorMoneySupplyID, state.Predicate{}, &BillData{
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
		err := m.revertibleState.UpdateData(tx.UnitId(), func(data state.UnitData) (newData state.UnitData) {
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
	return m.revertibleState.GetRootHash()
}

// TotalValue starts tree calculation and returns the root node monetary value.
// It must remain constant during the lifetime of the state.
func (m *moneySchemeState) TotalValue() (uint64, error) {
	sum := m.revertibleState.TotalValue()
	bs, ok := sum.(*BillSummary)
	if !ok {
		return 0, errors.New("summary was not *BillSummary")
	}
	return bs.v, nil
}

func (b *BillSummary) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(b.v))
}

func (b *BillSummary) Concatenate(left, right state.SummaryValue) state.SummaryValue {
	var out uint64
	out += b.v
	if left != nil {
		if ls, ok := left.(*BillSummary); ok {
			out += ls.v
		}
	}
	if right != nil {
		if rs, ok := right.(*BillSummary); ok {
			out += rs.v
		}
	}
	return &BillSummary{v: out}
}

func (b *BillData) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(b.V))
	hasher.Write(util.Uint64ToBytes(b.T))
	hasher.Write(b.Backlink)
}

func (b *BillData) Value() state.SummaryValue {
	return &BillSummary{v: b.V}
}
