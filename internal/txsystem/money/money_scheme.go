package money

import (
	"crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	txutil "gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/util"

	abHasher "gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"github.com/holiman/uint256"
)

// TODO move to the genesis file?
const dustBillDeletionTimeout uint64 = 300

type (
	Transfer interface {
		transaction.GenericTransaction
		NewBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	TransferDC interface {
		transaction.GenericTransaction
		Nonce() []byte
		TargetBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	Split interface {
		transaction.GenericTransaction
		Amount() uint64
		TargetBearer() []byte
		RemainingValue() uint64
		Backlink() []byte
		HashForIdCalculation(hashFunc crypto.Hash) []byte // Returns hash value for the sameShardId function
	}

	Swap interface {
		transaction.GenericTransaction
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
		GetUnit(id *uint256.Int) (*state.Unit, error)
		Revert()
		Commit()
		GetRootHash() []byte
		TotalValue() state.SummaryValue
		GetBlockNumber() uint64
	}

	moneySchemeState struct {
		systemIdentifier []byte
		revertibleState  RevertibleState
		hashAlgorithm    crypto.Hash // hash function algorithm

		// Contains the bill identifiers transferred to the dust collector. The key of the map is the block number when the
		// bill is deleted and its value is transferred to the dust collector.
		dustCollectorBills map[uint64][]*uint256.Int
	}
)

var (
	log = logger.CreateForPackage()

	// The ID of the dust collector money supply
	dustCollectorMoneySupplyID = uint256.NewInt(0)

	// Dust collector predicate
	dustCollectorPredicate = script.PredicatePayToPublicKeyHash(script.HashAlgSha256, abHasher.Sum256([]byte("dust collector")), script.SigSchemeSecp256k1)

	ErrInitialBillIsNil     = errors.New("initial bill may not be nil")
	ErrInvalidInitialBillID = errors.New("initial bill ID may not be equal to the DC money supply ID")
)

func NewMoneySchemeState(hashAlgorithm crypto.Hash, trustBase []string, initialBill *InitialBill, dcMoneyAmount uint64, customOpts ...MoneySchemeOption) (*moneySchemeState, error) {
	if initialBill == nil {
		return nil, ErrInitialBillIsNil
	}
	if dustCollectorMoneySupplyID.Eq(initialBill.ID) {
		return nil, ErrInvalidInitialBillID
	}
	defaultTree, err := state.New(&state.Config{HashAlgorithm: hashAlgorithm, TrustBase: trustBase})
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
		systemIdentifier:   []byte{0}, // TODO AB-178 get system identifier somewhere
		hashAlgorithm:      hashAlgorithm,
		revertibleState:    options.revertibleState,
		dustCollectorBills: make(map[uint64][]*uint256.Int),
	}

	err = msState.revertibleState.AddItem(initialBill.ID, initialBill.Owner, &BillData{
		V:        initialBill.Value,
		T:        0,
		Backlink: nil,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not set initial bill")
	}

	err = msState.revertibleState.AddItem(dustCollectorMoneySupplyID, dustCollectorPredicate, &BillData{
		V:        dcMoneyAmount,
		T:        0,
		Backlink: nil,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not set DC monet supply")
	}
	return msState, nil
}

func (m *moneySchemeState) Process(gtx transaction.GenericTransaction) error {
	bd, _ := m.revertibleState.GetUnit(gtx.UnitID())
	err := txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{Tx: gtx, Bd: bd, SystemIdentifier: m.systemIdentifier, BlockNumber: m.revertibleState.GetBlockNumber()})
	if err != nil {
		return err
	}
	switch tx := gtx.(type) {
	case Transfer:
		log.Debug("Processing transfer %v", tx)
		err := m.validateTransferTx(tx)
		if err != nil {
			return err
		}
		err = m.updateBillData(tx)
		if err != nil {
			return err
		}
		return m.revertibleState.SetOwner(tx.UnitID(), tx.NewBearer(), tx.Hash(m.hashAlgorithm))
	case TransferDC:
		log.Debug("Processing transferDC %v", tx)
		err := m.validateTransferDCTx(tx)
		if err != nil {
			return err
		}
		err = m.updateBillData(tx)
		if err != nil {
			return err
		}
		err = m.revertibleState.SetOwner(tx.UnitID(), dustCollectorPredicate, tx.Hash(m.hashAlgorithm))
		if err != nil {
			return err
		}
		delBlockNr := m.revertibleState.GetBlockNumber() + dustBillDeletionTimeout
		dustBillsArray := m.dustCollectorBills[delBlockNr]
		m.dustCollectorBills[delBlockNr] = append(dustBillsArray, tx.UnitID())
		return nil
	case Split:
		log.Debug("Processing split %v", tx)
		err := m.validateSplitTx(tx)
		if err != nil {
			return err
		}
		err = m.revertibleState.UpdateData(tx.UnitID(), func(data state.UnitData) (newData state.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				// No change in case of incorrect data type.
				return data
			}
			return &BillData{
				V:        bd.V - tx.Amount(),
				T:        m.revertibleState.GetBlockNumber(),
				Backlink: tx.Hash(m.hashAlgorithm),
			}
		}, tx.Hash(m.hashAlgorithm))
		if err != nil {
			return errors.Wrap(err, "could not update data")
		}

		newItemId := txutil.SameShardId(tx.UnitID(), tx.HashForIdCalculation(m.hashAlgorithm))
		err = m.revertibleState.AddItem(newItemId, tx.TargetBearer(), &BillData{
			V:        tx.Amount(),
			T:        m.revertibleState.GetBlockNumber(),
			Backlink: tx.Hash(m.hashAlgorithm),
		}, tx.Hash(m.hashAlgorithm))
		if err != nil {
			return errors.Wrap(err, "could not add item")
		}
	case Swap:
		log.Debug("Processing swap %v", tx)
		err := m.validateSwapTx(tx)
		if err != nil {
			return err
		}

		// set n as the target value
		n := tx.TargetValue()

		// reduce dc-money supply by n
		err = m.revertibleState.UpdateData(dustCollectorMoneySupplyID, func(data state.UnitData) (newData state.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				return bd
			}
			bd.V -= n
			return bd
		}, []byte{})
		if err != nil {
			return err
		}

		// create a new bill with value n and owner condition a
		err = m.revertibleState.AddItem(tx.UnitID(), tx.OwnerCondition(), &BillData{
			V:        n,
			T:        m.revertibleState.GetBlockNumber(),
			Backlink: tx.Hash(m.hashAlgorithm),
		}, tx.Hash(m.hashAlgorithm))
		if err != nil {
			return errors.Wrap(err, "could not add item")
		}
		return nil
	default:
		return errors.Errorf("Unknown type %T", gtx)
	}
	return nil
}

// EndBlock deletes dust bills from the state tree.
// TODO this function must be called by the "blockchain" component: AB-62 (block finalization)
func (m *moneySchemeState) EndBlock(blockNr uint64) error {
	dustBills := m.dustCollectorBills[blockNr]
	var valueToTransfer uint64
	for _, billID := range dustBills {
		u, err := m.revertibleState.GetUnit(billID)
		if err != nil {
			return err
		}
		bd, ok := u.Data.(*BillData)
		if !ok {
			// it is safe to ignore the data because it is not a bill
			continue
		}
		valueToTransfer += bd.V
		err = m.revertibleState.DeleteItem(billID)
		if err != nil {
			return err
		}
	}
	if valueToTransfer > 0 {
		err := m.revertibleState.UpdateData(dustCollectorMoneySupplyID, func(data state.UnitData) (newData state.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				return bd
			}
			bd.V += valueToTransfer
			return bd
		}, []byte{})
		if err != nil {
			return err
		}
	}
	delete(m.dustCollectorBills, blockNr)
	return nil
}

func (m *moneySchemeState) updateBillData(tx transaction.GenericTransaction) error {
	return m.revertibleState.UpdateData(tx.UnitID(), func(data state.UnitData) (newData state.UnitData) {
		bd, ok := data.(*BillData)
		if !ok {
			// No change in case of incorrect data type.
			return data
		}
		bd.T = m.revertibleState.GetBlockNumber()
		bd.Backlink = tx.Hash(m.hashAlgorithm)
		return bd
	}, tx.Hash(m.hashAlgorithm))
}

func (m *moneySchemeState) validateTransferTx(tx Transfer) error {
	data, err := m.revertibleState.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateTransfer(data.Data, tx)
}

func (m *moneySchemeState) validateTransferDCTx(tx TransferDC) error {
	data, err := m.revertibleState.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateTransferDC(data.Data, tx)
}

func (m *moneySchemeState) validateSplitTx(tx Split) error {
	data, err := m.revertibleState.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateSplit(data.Data, tx)
}

func (m *moneySchemeState) validateSwapTx(tx Swap) error {
	// 2. there is suffiecient DC-money supply
	dcMoneySupply, err := m.revertibleState.GetUnit(dustCollectorMoneySupplyID)
	if err != nil {
		return err
	}
	dcMoneySupplyBill, ok := dcMoneySupply.Data.(*BillData)
	if !ok {
		return txsystem.ErrInvalidDataType
	}
	if dcMoneySupplyBill.V < tx.TargetValue() {
		return ErrSwapInsufficientDCMoneySupply
	}
	// 3.there exists no bill with identifier
	_, err = m.revertibleState.GetUnit(tx.UnitID())
	if err == nil {
		return ErrSwapBillAlreadyExists
	}
	return validateSwap(tx, m.hashAlgorithm)
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

func (b *BillSummary) Bytes() []byte {
	return util.Uint64ToBytes(b.v)
}

func (b *BillData) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(b.V))
	hasher.Write(util.Uint64ToBytes(b.T))
	hasher.Write(b.Backlink)
}

func (b *BillData) Value() state.SummaryValue {
	return &BillSummary{v: b.V}
}
