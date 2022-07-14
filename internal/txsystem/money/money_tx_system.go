package money

import (
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/errors"
	abHasher "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const dustBillDeletionTimeout uint64 = 300

var (
	log = logger.CreateForPackage()

	// The ID of the dust collector money supply
	dustCollectorMoneySupplyID = uint256.NewInt(0)

	// Dust collector predicate
	dustCollectorPredicate = script.PredicatePayToPublicKeyHash(script.HashAlgSha256, abHasher.Sum256([]byte("dust collector")), script.SigSchemeSecp256k1)

	ErrInitialBillIsNil     = errors.New("initial bill may not be nil")
	ErrInvalidInitialBillID = errors.New("initial bill ID may not be equal to the DC money supply ID")
)

type (
	Transfer interface {
		txsystem.GenericTransaction
		NewBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	TransferDC interface {
		txsystem.GenericTransaction
		Nonce() []byte
		TargetBearer() []byte
		TargetValue() uint64
		Backlink() []byte
	}

	Split interface {
		txsystem.GenericTransaction
		Amount() uint64
		TargetBearer() []byte
		RemainingValue() uint64
		Backlink() []byte
		HashForIdCalculation(hashFunc crypto.Hash) []byte // Returns hash value for the sameShardId function
	}

	Swap interface {
		txsystem.GenericTransaction
		OwnerCondition() []byte
		BillIdentifiers() []*uint256.Int
		DCTransfers() []TransferDC
		Proofs() [][]byte
		TargetValue() uint64
	}

	InitialBill struct {
		ID    *uint256.Int
		Value uint64
		Owner rma.Predicate
	}

	BillData struct {
		V        uint64 // The monetary value of this bill
		T        uint64 // The round number of the last transaction with the bill
		Backlink []byte // Backlink (256-bit hash)
	}

	moneyTxSystem struct {
		systemIdentifier   []byte
		revertibleState    *rma.Tree
		hashAlgorithm      crypto.Hash // hash function algorithm
		currentBlockNumber uint64
		// Contains the bill identifiers transferred to the dust collector. The key of the map is the block number when the
		// bill is deleted and its value is transferred to the dust collector.
		dustCollectorBills map[uint64][]*uint256.Int
	}
)

func NewMoneyTxSystem(hashAlgorithm crypto.Hash, initialBill *InitialBill, dcMoneyAmount uint64, customOpts ...Option) (*moneyTxSystem, error) {
	if initialBill == nil {
		return nil, ErrInitialBillIsNil
	}
	if dustCollectorMoneySupplyID.Eq(initialBill.ID) {
		return nil, ErrInvalidInitialBillID
	}
	defaultTree, err := rma.New(&rma.Config{HashAlgorithm: hashAlgorithm})
	if err != nil {
		return nil, err
	}
	options := Options{
		revertibleState:  defaultTree,
		systemIdentifier: []byte{0, 0, 0, 0},
	}
	for _, o := range customOpts {
		o(&options)
	}

	txs := &moneyTxSystem{
		systemIdentifier:   options.systemIdentifier,
		hashAlgorithm:      hashAlgorithm,
		revertibleState:    options.revertibleState,
		dustCollectorBills: make(map[uint64][]*uint256.Int),
		currentBlockNumber: uint64(0),
	}

	err = txs.revertibleState.AddItem(initialBill.ID, initialBill.Owner, &BillData{
		V:        initialBill.Value,
		T:        0,
		Backlink: nil,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not set initial bill")
	}

	err = txs.revertibleState.AddItem(dustCollectorMoneySupplyID, dustCollectorPredicate, &BillData{
		V:        dcMoneyAmount,
		T:        0,
		Backlink: nil,
	}, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not set DC money supply")
	}
	txs.Commit()
	return txs, nil
}

func (m *moneyTxSystem) Execute(gtx txsystem.GenericTransaction) error {
	bd, _ := m.revertibleState.GetUnit(gtx.UnitID())
	err := txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{Tx: gtx, Bd: bd, SystemIdentifier: m.systemIdentifier, BlockNumber: m.currentBlockNumber})
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
		delBlockNr := m.currentBlockNumber + dustBillDeletionTimeout
		dustBillsArray := m.dustCollectorBills[delBlockNr]
		m.dustCollectorBills[delBlockNr] = append(dustBillsArray, tx.UnitID())
		return nil
	case Split:
		log.Debug("Processing split %v", tx)
		err := m.validateSplitTx(tx)
		if err != nil {
			return err
		}
		err = m.revertibleState.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				// No change in case of incorrect data type.
				return data
			}
			return &BillData{
				V:        bd.V - tx.Amount(),
				T:        m.currentBlockNumber,
				Backlink: tx.Hash(m.hashAlgorithm),
			}
		}, tx.Hash(m.hashAlgorithm))
		if err != nil {
			return errors.Wrap(err, "could not update data")
		}

		newItemId := txutil.SameShardId(tx.UnitID(), tx.HashForIdCalculation(m.hashAlgorithm))
		err = m.revertibleState.AddItem(newItemId, tx.TargetBearer(), &BillData{
			V:        tx.Amount(),
			T:        m.currentBlockNumber,
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

		// TODO verify ledger proofs AB-211

		// reduce dc-money supply by n
		err = m.revertibleState.UpdateData(dustCollectorMoneySupplyID, func(data rma.UnitData) (newData rma.UnitData) {
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
			T:        m.currentBlockNumber,
			Backlink: tx.Hash(m.hashAlgorithm),
		}, tx.Hash(m.hashAlgorithm))
		if err != nil {
			return errors.Wrap(err, "could not add item")
		}
		return nil
	default:
		return errors.Errorf("unknown type %T", gtx)
	}
	return nil
}

func (m *moneyTxSystem) State() (txsystem.State, error) {
	if m.revertibleState.ContainsUncommittedChanges() {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	return txsystem.NewStateSummary(
		m.revertibleState.GetRootHash(),
		m.revertibleState.TotalValue().Bytes(),
	), nil
}

func (m *moneyTxSystem) BeginBlock(blockNr uint64) {
	m.currentBlockNumber = blockNr
}

func (m *moneyTxSystem) Revert() {
	m.revertibleState.Revert()
}

func (m *moneyTxSystem) Commit() {
	m.revertibleState.Commit()
}

func (m *moneyTxSystem) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return NewMoneyTx(m.systemIdentifier, tx)
}

// EndBlock deletes dust bills from the state tree.
func (m *moneyTxSystem) EndBlock() (txsystem.State, error) {
	dustBills := m.dustCollectorBills[m.currentBlockNumber]
	var valueToTransfer uint64
	for _, billID := range dustBills {
		u, err := m.revertibleState.GetUnit(billID)
		if err != nil {
			return nil, err
		}
		bd, ok := u.Data.(*BillData)
		if !ok {
			// it is safe to ignore the data because it is not a bill
			continue
		}
		valueToTransfer += bd.V
		err = m.revertibleState.DeleteItem(billID)
		if err != nil {
			return nil, err
		}
	}
	if valueToTransfer > 0 {
		err := m.revertibleState.UpdateData(dustCollectorMoneySupplyID, func(data rma.UnitData) (newData rma.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				return bd
			}
			bd.V += valueToTransfer
			return bd
		}, []byte{})
		if err != nil {
			return nil, err
		}
	}
	delete(m.dustCollectorBills, m.currentBlockNumber)
	return txsystem.NewStateSummary(
		m.revertibleState.GetRootHash(),
		m.revertibleState.TotalValue().Bytes(),
	), nil
}

func (m *moneyTxSystem) updateBillData(tx txsystem.GenericTransaction) error {
	return m.revertibleState.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
		bd, ok := data.(*BillData)
		if !ok {
			// No change in case of incorrect data type.
			return data
		}
		bd.T = m.currentBlockNumber
		bd.Backlink = tx.Hash(m.hashAlgorithm)
		return bd
	}, tx.Hash(m.hashAlgorithm))
}

func (m *moneyTxSystem) validateTransferTx(tx Transfer) error {
	data, err := m.revertibleState.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateTransfer(data.Data, tx)
}

func (m *moneyTxSystem) validateTransferDCTx(tx TransferDC) error {
	data, err := m.revertibleState.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateTransferDC(data.Data, tx)
}

func (m *moneyTxSystem) validateSplitTx(tx Split) error {
	data, err := m.revertibleState.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	return validateSplit(data.Data, tx)
}

func (m *moneyTxSystem) validateSwapTx(tx Swap) error {
	// 2. there is sufficient DC-money supply
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
func (m *moneyTxSystem) GetRootHash() []byte {
	return m.revertibleState.GetRootHash()
}

// TotalValue starts tree calculation and returns the root node monetary value.
// It must remain constant during the lifetime of the state.
func (m *moneyTxSystem) TotalValue() (uint64, error) {
	sum := m.revertibleState.TotalValue()
	bs, ok := sum.(*rma.Uint64SummaryValue)
	if !ok {
		return 0, errors.New("summary was not *BillSummary")
	}
	return bs.Value(), nil
}

func (b *BillData) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(b.V))
	hasher.Write(util.Uint64ToBytes(b.T))
	hasher.Write(b.Backlink)
}

func (b *BillData) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(b.V)
}
