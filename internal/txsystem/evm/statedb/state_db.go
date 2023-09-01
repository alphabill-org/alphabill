package statedb

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/alphabill-org/alphabill/pkg/tree/avl"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
)

var _ vm.StateDB = (*StateDB)(nil)
var log = logger.CreateForPackage()

type (
	revision struct {
		id         int
		journalIdx int
	}

	StateDB struct {
		tree       *state.State
		errDB      error
		accessList *accessList

		suicides []common.Address
		logs     []*LogEntry
		// The refund counter, also used by state transitioning.
		refund uint64
		// track changes
		journal   *journal
		revisions []revision
	}

	LogEntry struct {
		_       struct{} `cbor:",toarray"`
		Address common.Address
		Topics  []common.Hash
		Data    []byte
	}
)

func NewStateDB(tree *state.State) *StateDB {
	return &StateDB{
		tree:       tree,
		accessList: newAccessList(),
		journal:    newJournal(),
	}
}

func (s *StateDB) CreateAccount(address common.Address) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil {
		// TODO handle the case when the account is overridden!
		// It should be enough to keep the balance and set nonce to 0
		return
	}
	log.Trace("Adding an account: %v", address)
	s.errDB = s.tree.Apply(state.AddUnit(
		unitID,
		script.PredicateAlwaysFalse(),
		&StateObject{Address: address, Account: &Account{Nonce: 0, Balance: big.NewInt(0), CodeHash: emptyCodeHash}, Storage: map[common.Hash]common.Hash{}},
	))
	if s.errDB == nil {
		s.journal.append(&address)
	}
}

func (s *StateDB) SubBalance(address common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	log.Trace("SubBalance: account %v, initial balance %v, amount to subtract %v", address, stateObject.Account.Balance, amount)
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) state.UnitData {
		newBalance := new(big.Int).Sub(so.Account.Balance, amount)
		so.Account.Balance = newBalance
		return so
	})
}

func (s *StateDB) AddBalance(address common.Address, amount *big.Int) {
	if amount.Sign() == 0 {
		return
	}
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	log.Trace("AddBalance: account %v, initial balance %v, amount to add %v", address, stateObject.Account.Balance, amount)
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) state.UnitData {
		newBalance := new(big.Int).Add(so.Account.Balance, amount)
		so.Account.Balance = newBalance
		return so
	})
}

func (s *StateDB) GetBalance(address common.Address) *big.Int {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil {
		return stateObject.Account.Balance
	}
	return big.NewInt(0)
}

func (s *StateDB) GetNonce(address common.Address) uint64 {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil {
		return stateObject.Account.Nonce
	}
	return 0
}

func (s *StateDB) SetNonce(address common.Address, nonce uint64) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	log.Trace("Setting a new nonce %v for an account: %v", nonce, address)
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) state.UnitData {
		so.Account.Nonce = nonce
		return so
	})
}

func (s *StateDB) GetCodeHash(address common.Address) common.Hash {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil {
		return common.BytesToHash(stateObject.Account.CodeHash)
	}
	return common.Hash{}
}

func (s *StateDB) GetCode(address common.Address) []byte {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil {
		return stateObject.Account.Code
	}
	return nil
}

func (s *StateDB) SetCode(address common.Address, code []byte) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	log.Trace("Setting code %X for an account: %v", code, address)
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) state.UnitData {
		so.Account.Code = code
		so.Account.CodeHash = crypto.Keccak256Hash(code).Bytes()
		return so
	})
}

func (s *StateDB) GetCodeSize(address common.Address) int {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil {
		return len(stateObject.Account.Code)
	}
	return 0
}

func (s *StateDB) AddRefund(gas uint64) {
	s.refund += gas
}

func (s *StateDB) SubRefund(gas uint64) {
	if gas > s.refund {
		panic(fmt.Sprintf("Refund counter below zero (gas: %d > refund: %d)", gas, s.refund))
	}
	s.refund -= gas
}

func (s *StateDB) GetRefund() uint64 {
	return s.refund
}

func (s *StateDB) GetCommittedState(address common.Address, key common.Hash) common.Hash {
	stateObject := s.getStateObject(address.Bytes(), true)
	if stateObject == nil {
		return common.Hash{}
	}
	return stateObject.Storage[key]
}

func (s *StateDB) GetState(address common.Address, key common.Hash) common.Hash {
	stateObject := s.getStateObject(address.Bytes(), false)
	if stateObject == nil {
		return common.Hash{}
	}
	return stateObject.Storage[key]
}

func (s *StateDB) SetState(address common.Address, key common.Hash, value common.Hash) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	log.Trace("Setting a state (key=%v, value=%v) for an account: %v", key, value, address)
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) state.UnitData {
		so.Storage[key] = value
		return so
	})
}

func (s *StateDB) Suicide(address common.Address) bool {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return false
	}
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) state.UnitData {
		so.suicided = true
		so.Account.Balance = big.NewInt(0)
		s.suicides = append(s.suicides, address)
		return so
	})
	if s.errDB == nil {
		s.journal.append(&address)
	}
	return true
}

func (s *StateDB) HasSuicided(address common.Address) bool {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return false
	}
	return stateObject.suicided
}

func (s *StateDB) Exist(address common.Address) bool {
	so := s.getStateObject(address.Bytes(), false)
	return so != nil
}

func (s *StateDB) Empty(address common.Address) bool {
	so := s.getStateObject(address.Bytes(), false)
	return so == nil || so.empty()
}

// PrepareAccessList handles the preparatory steps for executing a state transition with
// regards to both EIP-2929 and EIP-2930:
//
// - Add sender to access list (2929)
// - Add destination to access list (2929)
// - Add precompiles to access list (2929)
// - Add the contents of the optional tx access list (2930)
//
// This method should only be called if Yolov3/Berlin/2929+2930 is applicable at the current number.
func (s *StateDB) PrepareAccessList(sender common.Address, dst *common.Address, precompiles []common.Address, list ethtypes.AccessList) {
	s.AddAddressToAccessList(sender)
	if dst != nil {
		s.AddAddressToAccessList(*dst)
		// If it's a create-tx, the destination will be added inside evm.create
	}
	for _, addr := range precompiles {
		s.AddAddressToAccessList(addr)
	}
	for _, el := range list {
		s.AddAddressToAccessList(el.Address)
		for _, key := range el.StorageKeys {
			s.AddSlotToAccessList(el.Address, key)
		}
	}
}

// AddAddressToAccessList adds the given address to the access list
func (s *StateDB) AddAddressToAccessList(addr common.Address) {
	s.accessList.AddAddress(addr)
}

// AddSlotToAccessList adds the given (address, slot)-tuple to the access list
func (s *StateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	s.accessList.AddSlot(addr, slot)
}

// AddressInAccessList returns true if the given address is in the access list.
func (s *StateDB) AddressInAccessList(addr common.Address) bool {
	return s.accessList.ContainsAddress(addr)
}

// SlotInAccessList returns true if the given (address, slot)-tuple is in the access list.
func (s *StateDB) SlotInAccessList(addr common.Address, slot common.Hash) (addressPresent bool, slotPresent bool) {
	return s.accessList.Contains(addr, slot)
}

func (s *StateDB) RevertToSnapshot(i int) {
	s.tree.RollbackToSavepoint(i)
	// remove reverted units
	for idx, rev := range s.revisions {
		if rev.id >= i {
			s.journal.revert(rev.journalIdx)
			s.revisions = s.revisions[:idx]
			return
		}
	}
}

func (s *StateDB) Snapshot() int {
	id := s.tree.Savepoint()
	s.revisions = append(s.revisions, revision{id, s.journal.length()})
	return id
}

func (s *StateDB) AddLog(log *ethtypes.Log) {
	if log == nil {
		return
	}
	// fields log.BlockNumber, log.TxHash, log.TxIndex, log.BlockHash and log.Index aren't initialized because of logs are stored inside a TransactionRecord.
	s.logs = append(s.logs, &LogEntry{
		Address: log.Address,
		Topics:  log.Topics,
		Data:    log.Data,
	})
}

func (s *StateDB) GetLogs() []*LogEntry {
	return s.logs
}

func (s *StateDB) AddPreimage(_ common.Hash, _ []byte) {
	// no-op! the EnablePreimageRecording flag is disabled in vm.Config
}

func (s *StateDB) ForEachStorage(address common.Address, f func(common.Hash, common.Hash) bool) error {
	so := s.getStateObject(address.Bytes(), true)
	if so == nil {
		return nil
	}

	for key, value := range so.Storage {
		if !f(key, value) {
			return nil
		}
	}
	return nil
}

func (s *StateDB) Finalize() error {
	var deletions []state.Action
	for _, address := range s.suicides {
		unitID := address.Bytes()
		so := s.getStateObject(unitID, false)
		if so == nil {
			continue
		}
		if so.suicided {
			deletions = append(deletions, state.DeleteUnit(unitID))
		}
	}
	if err := s.tree.Apply(deletions...); err != nil {
		return fmt.Errorf("unable to delete self destructed contract(s): %w", err)
	}
	s.suicides = nil
	s.logs = nil
	// clear unit tracking
	s.journal = newJournal()
	s.revisions = []revision{}
	return nil
}

func (s *StateDB) SetAlphaBillData(address common.Address, fee *AlphaBillLink) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	log.Trace("Setting fee credit data for an account: %v", address)
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) state.UnitData {
		so.AlphaBill = fee
		return so
	})
}

func (s *StateDB) GetAlphaBillData(address common.Address) *AlphaBillLink {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil && stateObject.AlphaBill != nil {
		return stateObject.AlphaBill
	}
	return nil
}

func (s *StateDB) getStateObject(unitID types.UnitID, committed bool) *StateObject {
	u, err := s.tree.GetUnit(unitID, committed)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return nil
		}
		s.errDB = err
	}
	return u.Data().(*StateObject)
}

func (s *StateDB) DBError() error {
	return s.errDB
}

// GetUpdatedUnits returns updated UnitID's since last Finalize call
func (s *StateDB) GetUpdatedUnits() []types.UnitID {
	addrs := s.journal.getModifiedUnits()
	units := make([]types.UnitID, len(addrs))
	i := 0
	for k := range addrs {
		units[i] = k.Bytes()
		i++
	}
	sort.Slice(units, func(i, j int) bool {
		return bytes.Compare(units[i], units[j]) < 0
	})
	return units
}

func (s *StateDB) executeUpdate(id types.UnitID, updateFunc func(so *StateObject) state.UnitData) error {
	if err := s.tree.Apply(state.UpdateUnitData(id, func(data state.UnitData) (state.UnitData, error) {
		so, ok := data.(*StateObject)
		if !ok {
			return so, fmt.Errorf("unit data is not an instance of StateObject")
		}
		return updateFunc(so), nil
	})); err != nil {
		return err
	}
	addr := common.BytesToAddress(id)
	s.journal.append(&addr)
	return nil
}
