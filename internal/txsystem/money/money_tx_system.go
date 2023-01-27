package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	abHasher "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/validator"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const dustBillDeletionTimeout uint64 = 65536

var (
	log = logger.CreateForPackage()

	// The ID of the dust collector money supply
	dustCollectorMoneySupplyID = uint256.NewInt(0)

	// Dust collector predicate
	dustCollectorPredicate = script.PredicatePayToPublicKeyHash(script.HashAlgSha256, abHasher.Sum256([]byte("dust collector")), script.SigSchemeSecp256k1)

	ErrInitialBillIsNil                  = errors.New("initial bill may not be nil")
	ErrInvalidInitialBillID              = errors.New("initial bill ID may not be equal to the DC money supply ID")
	ErrUndefinedSystemDescriptionRecords = errors.New("undefined system description records")
	ErrNilFeeCreditBill                  = errors.New("fee credit bill is nil in system description record")
	ErrInvalidFeeCreditBillID            = errors.New("fee credit bill may not be equal to the DC money supply ID and initial bill ID")
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
		Proofs() []*block.BlockProof
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

	FeeCreditTxValidator interface {
		ValidateAddFC(ctx *validator.AddFCValidationContext) error
		ValidateCloseFC(ctx *validator.CloseFCValidationContext) error
	}

	moneyTxSystem struct {
		systemIdentifier   []byte
		revertibleState    *rma.Tree
		hashAlgorithm      crypto.Hash // hash function algorithm
		currentBlockNumber uint64
		// Contains the bill identifiers transferred to the dust collector. The key of the map is the block number when the
		// bill is deleted and its value is transferred to the dust collector.
		dustCollectorBills map[uint64][]*uint256.Int
		trustBase          map[string]abcrypto.Verifier
		// sdrs system description records indexed by string(system_identifier)
		sdrs map[string]*genesis.SystemDescriptionRecord
		// feeCreditTxRecorder recorded fee credit transactions in current round
		feeCreditTxRecorder *feeCreditTxRecorder
		// feeCreditTxValidator validator for partition specific AddFC and CloseFC fee credit transactions
		feeCreditTxValidator FeeCreditTxValidator
	}
)

func NewMoneyTxSystem(hashAlgorithm crypto.Hash, initialBill *InitialBill, sdrs []*genesis.SystemDescriptionRecord, dcMoneyAmount uint64, customOpts ...Option) (*moneyTxSystem, error) {
	if initialBill == nil {
		return nil, ErrInitialBillIsNil
	}
	if dustCollectorMoneySupplyID.Eq(initialBill.ID) {
		return nil, ErrInvalidInitialBillID
	}
	if len(sdrs) == 0 {
		return nil, ErrUndefinedSystemDescriptionRecords
	}
	defaultTree, err := rma.New(&rma.Config{HashAlgorithm: hashAlgorithm})
	if err != nil {
		return nil, err
	}
	options := Options{
		revertibleState:  defaultTree,
		systemIdentifier: []byte{0, 0, 0, 0},
		trustBase:        make(map[string]abcrypto.Verifier),
	}
	for _, o := range customOpts {
		o(&options)
	}

	txs := &moneyTxSystem{
		systemIdentifier:     options.systemIdentifier,
		hashAlgorithm:        hashAlgorithm,
		revertibleState:      options.revertibleState,
		dustCollectorBills:   make(map[uint64][]*uint256.Int),
		currentBlockNumber:   uint64(0),
		trustBase:            options.trustBase,
		sdrs:                 make(map[string]*genesis.SystemDescriptionRecord),
		feeCreditTxValidator: validator.NewDefaultFeeCreditTxValidator(options.systemIdentifier, options.systemIdentifier, hashAlgorithm, options.trustBase),
	}

	err = txs.revertibleState.AtomicUpdate(rma.AddItem(initialBill.ID, initialBill.Owner, &BillData{
		V:        initialBill.Value,
		T:        0,
		Backlink: nil,
	}, nil))
	if err != nil {
		return nil, fmt.Errorf("could not set initial bill: %w", err)
	}

	// add fee credit bills to state tree
	for _, sdr := range sdrs {
		feeCreditBill := sdr.FeeCreditBill
		if feeCreditBill == nil {
			return nil, ErrNilFeeCreditBill
		}
		if bytes.Equal(feeCreditBill.UnitId, util.Uint256ToBytes(dustCollectorMoneySupplyID)) || bytes.Equal(feeCreditBill.UnitId, util.Uint256ToBytes(initialBill.ID)) {
			return nil, ErrInvalidFeeCreditBillID
		}
		err = txs.revertibleState.AtomicUpdate(rma.AddItem(uint256.NewInt(0).SetBytes(feeCreditBill.UnitId), feeCreditBill.OwnerPredicate, &BillData{
			V:        0,
			T:        0,
			Backlink: nil,
		}, nil))
		if err != nil {
			return nil, fmt.Errorf("could not set fee credit bill: %w", err)
		}
		txs.sdrs[string(sdr.SystemIdentifier)] = sdr
	}

	err = txs.revertibleState.AtomicUpdate(rma.AddItem(dustCollectorMoneySupplyID, dustCollectorPredicate, &BillData{
		V:        dcMoneyAmount,
		T:        0,
		Backlink: nil,
	}, nil))
	if err != nil {
		return nil, fmt.Errorf("could not set DC money supply: %w", err)
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
		return m.revertibleState.AtomicUpdate(rma.SetOwner(tx.UnitID(), tx.NewBearer(), tx.Hash(m.hashAlgorithm)))
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
		err = m.revertibleState.AtomicUpdate(rma.SetOwner(tx.UnitID(), dustCollectorPredicate, tx.Hash(m.hashAlgorithm)))
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
		h := tx.Hash(m.hashAlgorithm)
		newItemId := txutil.SameShardID(tx.UnitID(), tx.HashForIdCalculation(m.hashAlgorithm))
		return m.revertibleState.AtomicUpdate(
			rma.UpdateData(tx.UnitID(),
				func(data rma.UnitData) (newData rma.UnitData) {
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
				}, h),
			rma.AddItem(newItemId, tx.TargetBearer(), &BillData{
				V:        tx.Amount(),
				T:        m.currentBlockNumber,
				Backlink: tx.Hash(m.hashAlgorithm),
			}, h))
	case Swap:
		log.Debug("Processing swap %v", tx)
		err := m.validateSwapTx(tx)
		if err != nil {
			return err
		}
		// set n as the target value
		n := tx.TargetValue()
		// reduce dc-money supply by n
		decDustCollectorSupplyFn := func(data rma.UnitData) (newData rma.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				return bd
			}
			bd.V -= n
			return bd
		}
		return m.revertibleState.AtomicUpdate(
			rma.UpdateData(dustCollectorMoneySupplyID, decDustCollectorSupplyFn, []byte{}),
			rma.AddItem(tx.UnitID(), tx.OwnerCondition(), &BillData{
				V:        n,
				T:        m.currentBlockNumber,
				Backlink: tx.Hash(m.hashAlgorithm),
			}, tx.Hash(m.hashAlgorithm)))
	case *fc.TransferFeeCreditWrapper:
		log.Debug("Processing transferFC %v", tx)
		if bd == nil {
			return errors.New("transferFC: unit not found")
		}
		bdd, ok := bd.Data.(*BillData)
		if !ok {
			return errors.New("transferFC: invalid unit type")
		}
		err = validateTransferFC(tx, bdd)
		if err != nil {
			return fmt.Errorf("transferFC: validation failed: %w", err)
		}

		// calculate actual tx fee cost
		tx.Transaction.ServerMetadata.Fee = txFeeFunc()

		// remove value from source unit, or delete source bill entirely
		var updateFunc rma.Action
		v := tx.TransferFC.Amount + tx.Transaction.ServerMetadata.Fee
		if v < bdd.V {
			dataUpdateFunc := func(data rma.UnitData) (newData rma.UnitData) {
				newBillData, ok := data.(*BillData)
				if !ok {
					return data // TODO should return error instead
				}
				newBillData.V -= v
				newBillData.T = m.currentBlockNumber
				newBillData.Backlink = tx.Hash(m.hashAlgorithm)
				return newBillData
			}
			updateFunc = rma.UpdateData(tx.UnitID(), dataUpdateFunc, tx.Hash(m.hashAlgorithm))
		} else {
			updateFunc = rma.DeleteItem(tx.UnitID())
		}
		err = m.revertibleState.AtomicUpdate(updateFunc)
		if err != nil {
			return fmt.Errorf("transferFC: failed to update state: %w", err)
		}
		// record fee tx for end of the round consolidation
		m.feeCreditTxRecorder.recordTransferFC(tx)
		return nil
	case *fc.AddFeeCreditWrapper:
		log.Debug("Processing addFC %v", tx)
		err = m.feeCreditTxValidator.ValidateAddFC(&validator.AddFCValidationContext{
			Tx:                 tx,
			Unit:               bd,
			CurrentRoundNumber: m.currentBlockNumber,
		})
		if err != nil {
			return fmt.Errorf("addFC tx validation failed: %w", err)
		}
		// calculate actual tx fee cost
		tx.Transaction.ServerMetadata.Fee = txFeeFunc()

		var updateFunc rma.Action
		// find net value of credit
		v := tx.TransferFC.TransferFC.Amount - tx.Transaction.ServerMetadata.Fee
		if bd == nil {
			// add credit
			fcr := &txsystem.FeeCreditRecord{
				Balance: v,
				Hash:    tx.Hash(m.hashAlgorithm),
				Timeout: tx.TransferFC.TransferFC.LatestAdditionTime + 1,
			}
			updateFunc = txsystem.AddCredit(tx.UnitID(), tx.AddFC.FeeCreditOwnerCondition, fcr, tx.Hash(m.hashAlgorithm))
		} else {
			// increment credit
			updateFunc = txsystem.IncrCredit(tx.UnitID(), v, tx.TransferFC.TransferFC.LatestAdditionTime+1, tx.Hash(m.hashAlgorithm))
		}
		err = m.revertibleState.AtomicUpdate(updateFunc)
		if err != nil {
			return fmt.Errorf("addFC state update failed: %w", err)
		}
		return nil
	case *fc.CloseFeeCreditWrapper:
		log.Debug("Processing closeFC %v", tx)
		err = m.feeCreditTxValidator.ValidateCloseFC(&validator.CloseFCValidationContext{
			Tx:   tx,
			Unit: bd,
		})
		if err != nil {
			return fmt.Errorf("closeFC: tx validation failed: %w", err)
		}
		// calculate actual tx fee cost
		tx.Transaction.ServerMetadata.Fee = txFeeFunc()

		// decrement credit
		updateFunc := txsystem.DecrCredit(tx.UnitID(), tx.CloseFC.Amount, tx.Hash(m.hashAlgorithm))
		err = m.revertibleState.AtomicUpdate(updateFunc)
		if err != nil {
			return fmt.Errorf("closeFC: state update failed: %w", err)
		}
		return nil
	case *fc.ReclaimFeeCreditWrapper:
		log.Debug("Processing reclaimFC %v", tx)
		if bd == nil {
			return errors.New("reclaimFC: unit not found")
		}
		bdd, ok := bd.Data.(*BillData)
		if !ok {
			return errors.New("reclaimFC: invalid unit type")
		}
		err = validateReclaimFC(tx, bdd, m.trustBase, m.hashAlgorithm)
		if err != nil {
			return fmt.Errorf("reclaimFC: validation failed: %w", err)
		}

		// calculate actual tx fee cost
		tx.Transaction.ServerMetadata.Fee = txFeeFunc()

		// add reclaimed value to source unit
		v := tx.CloseFCTransfer.CloseFC.Amount - tx.CloseFCTransfer.Transaction.ServerMetadata.Fee - tx.Transaction.ServerMetadata.Fee
		updateFunc := func(data rma.UnitData) (newData rma.UnitData) {
			newBillData, ok := data.(*BillData)
			if !ok {
				return data // TODO should return error instead
			}
			newBillData.V += v
			newBillData.T = m.currentBlockNumber
			newBillData.Backlink = tx.Hash(m.hashAlgorithm)
			return newBillData
		}
		updateAction := rma.UpdateData(tx.UnitID(), updateFunc, tx.Hash(m.hashAlgorithm))
		err = m.revertibleState.AtomicUpdate(updateAction)
		if err != nil {
			return fmt.Errorf("reclaimFC: failed to update state: %w", err)
		}
		m.feeCreditTxRecorder.recordReclaimFC(tx)
		return nil
	default:
		return fmt.Errorf("unknown type %T", gtx)
	}
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
	m.feeCreditTxRecorder = newFeeCreditTxRecorder()
}

func (m *moneyTxSystem) Revert() {
	m.revertibleState.Revert()
	m.feeCreditTxRecorder = nil
}

func (m *moneyTxSystem) Commit() {
	m.revertibleState.Commit()
	m.feeCreditTxRecorder = nil
}

func (m *moneyTxSystem) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return NewMoneyTx(m.systemIdentifier, tx)
}

// EndBlock deletes dust bills from the state tree and consolidates fee credits.
func (m *moneyTxSystem) EndBlock() (txsystem.State, error) {
	err := m.consolidateDust()
	if err != nil {
		return nil, err
	}
	err = m.consolidateFees()
	if err != nil {
		return nil, err
	}
	return txsystem.NewStateSummary(
		m.revertibleState.GetRootHash(),
		m.revertibleState.TotalValue().Bytes(),
	), nil
}

func (m *moneyTxSystem) consolidateDust() error {
	dustBills := m.dustCollectorBills[m.currentBlockNumber]
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
		err = m.revertibleState.AtomicUpdate(rma.DeleteItem(billID))
		if err != nil {
			return err
		}
	}
	if valueToTransfer > 0 {
		err := m.revertibleState.AtomicUpdate(rma.UpdateData(dustCollectorMoneySupplyID,
			func(data rma.UnitData) (newData rma.UnitData) {
				bd, ok := data.(*BillData)
				if !ok {
					return bd
				}
				bd.V += valueToTransfer
				return bd
			}, []byte{}))
		if err != nil {
			return err
		}
	}
	delete(m.dustCollectorBills, m.currentBlockNumber)
	return nil
}

func (m *moneyTxSystem) consolidateFees() error {
	// update fee credit bills for all known partitions with added and removed credits
	for sid, sdr := range m.sdrs {
		addedCredit := m.feeCreditTxRecorder.getAddedCredit(sid)
		reclaimedCredit := m.feeCreditTxRecorder.getReclaimedCredit(sid)
		if addedCredit == reclaimedCredit {
			continue // no update if bill value doesn't change
		}
		fcUnitID := uint256.NewInt(0).SetBytes(sdr.FeeCreditBill.UnitId)
		fcUnit, err := m.revertibleState.GetUnit(fcUnitID)
		if err != nil {
			return err
		}
		updateData := rma.UpdateData(fcUnitID,
			func(data rma.UnitData) (newData rma.UnitData) {
				bd, ok := data.(*BillData)
				if !ok {
					// TODO updateData should return error
					return data
				}
				bd.V = bd.V + addedCredit - reclaimedCredit
				return bd
			},
			fcUnit.StateHash)
		err = m.revertibleState.AtomicUpdate(updateData)
		if err != nil {
			return fmt.Errorf("failed to update [%x] partiton's fee credit bill: %w", sdr.SystemIdentifier, err)
		}
	}

	// increment money fee credit bill with spent fees
	spentFeeSum := m.feeCreditTxRecorder.getSpentFeeSum()
	if spentFeeSum > 0 {
		moneyFCUnitID := uint256.NewInt(0).SetBytes(m.sdrs[string(m.systemIdentifier)].FeeCreditBill.UnitId)
		moneyFCUnit, err := m.revertibleState.GetUnit(moneyFCUnitID)
		if err != nil {
			return fmt.Errorf("could not find money fee credit bill: %w", err)
		}
		updateData := rma.UpdateData(moneyFCUnitID,
			func(data rma.UnitData) (newData rma.UnitData) {
				bd, ok := data.(*BillData)
				if !ok {
					// TODO updateData should return error
					return data
				}
				bd.V = bd.V + spentFeeSum
				return bd
			},
			moneyFCUnit.StateHash)
		err = m.revertibleState.AtomicUpdate(updateData)
		if err != nil {
			return fmt.Errorf("failed to update money fee credit bill with spent fees: %w", err)
		}
	}
	return nil
}

func (m *moneyTxSystem) updateBillData(tx txsystem.GenericTransaction) error {
	return m.revertibleState.AtomicUpdate(rma.UpdateData(tx.UnitID(),
		func(data rma.UnitData) (newData rma.UnitData) {
			bd, ok := data.(*BillData)
			if !ok {
				// No change in case of incorrect data type.
				return data
			}
			bd.T = m.currentBlockNumber
			bd.Backlink = tx.Hash(m.hashAlgorithm)
			return bd
		}, tx.Hash(m.hashAlgorithm)))
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
	// 3. there is sufficient DC-money supply
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
	// 4.there exists no bill with identifier
	_, err = m.revertibleState.GetUnit(tx.UnitID())
	if err == nil {
		return ErrSwapBillAlreadyExists
	}
	return validateSwap(tx, m.hashAlgorithm, m.trustBase)
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

// txFeeFunc placeholder transaction cost function, all tx costs hardcoded to 1 (smallest?) alpha
func txFeeFunc() uint64 {
	return 1
}
