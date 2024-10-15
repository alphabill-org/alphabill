package statedb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/ethereum/go-ethereum/common"
	ethstate "github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/holiman/uint256"
)

var _ vm.StateDB = (*StateDB)(nil)

type (
	// transientStorage is a representation of EIP-1153 "Transient Storage".
	transientStorage map[common.Address]ethstate.Storage

	revision struct {
		id         int
		journalIdx int
	}

	StateDB struct {
		tree       *state.State
		errDB      error
		accessList *accessList

		suicides []common.Address
		logs     []*evm.LogEntry
		// The refund counter, also used by state transitioning.
		refund uint64
		// Transient storage
		transientStorage transientStorage
		// track changes
		journal   *journal
		revisions []revision
		created   map[common.Address]struct{}
		log       *slog.Logger
	}
)

// newTransientStorage creates a new instance of a transientStorage.
func newTransientStorage() transientStorage {
	return make(transientStorage)
}

// Set sets the transient-storage `value` for `key` at the given `addr`.
func (t transientStorage) Set(addr common.Address, key, value common.Hash) {
	if _, ok := t[addr]; !ok {
		t[addr] = make(ethstate.Storage)
	}
	t[addr][key] = value
}

// Get gets the transient storage for `key` at the given `addr`.
func (t transientStorage) Get(addr common.Address, key common.Hash) common.Hash {
	val, ok := t[addr]
	if !ok {
		return common.Hash{}
	}
	return val[key]
}

func NewStateDB(tree *state.State, log *slog.Logger) *StateDB {
	return &StateDB{
		tree:             tree,
		accessList:       newAccessList(),
		journal:          newJournal(),
		created:          map[common.Address]struct{}{},
		transientStorage: newTransientStorage(),
		log:              log,
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
	s.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("Adding an account: %v", address))
	unitData := &StateObject{Address: address, Account: &Account{Nonce: 0, Balance: uint256.NewInt(0), CodeHash: emptyCodeHash}, Storage: map[common.Hash]common.Hash{}}
	s.errDB = s.tree.Apply(state.AddUnit(unitID, unitData))
	if s.errDB == nil {
		s.created[address] = struct{}{}
		s.journal.append(accountChange{account: &address})
	}
}

func (s *StateDB) SubBalance(address common.Address, amount *uint256.Int, _ tracing.BalanceChangeReason) {
	if amount.Sign() == 0 {
		return
	}
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	s.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("SubBalance: account %v, initial balance %v, amount to subtract %v", address, stateObject.Account.Balance, amount))
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) types.UnitData {
		newBalance := new(uint256.Int).Sub(so.Account.Balance, amount)
		so.Account.Balance = newBalance
		return so
	})
}

func (s *StateDB) AddBalance(address common.Address, amount *uint256.Int, _ tracing.BalanceChangeReason) {
	if amount.Sign() == 0 {
		return
	}
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	s.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("AddBalance: account %v, initial balance %v, amount to add %v", address, stateObject.Account.Balance, amount))
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) types.UnitData {
		newBalance := new(uint256.Int).Add(so.Account.Balance, amount)
		so.Account.Balance = newBalance
		return so
	})
}

func (s *StateDB) GetBalance(address common.Address) *uint256.Int {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject != nil {
		return stateObject.Account.Balance
	}
	return uint256.NewInt(0)
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
	s.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("Setting a new nonce %v for an account: %v", nonce, address))
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) types.UnitData {
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
	s.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("Setting code %X for an account: %v", code, address))
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) types.UnitData {
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
	s.journal.append(refundChange{prev: s.refund})
	s.refund += gas
}

func (s *StateDB) SubRefund(gas uint64) {
	s.journal.append(refundChange{prev: s.refund})
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
	s.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("Setting a state (key=%v, value=%v) for an account: %v", key, value, address))
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) types.UnitData {
		so.Storage[key] = value
		return so
	})
}

func (s *StateDB) GetStorageRoot(addr common.Address) common.Hash {
	// todo: investigate if this needs to be supported
	return common.Hash{}
}

// GetTransientState gets transient storage for a given account.
func (s *StateDB) GetTransientState(addr common.Address, key common.Hash) common.Hash {
	return s.transientStorage.Get(addr, key)
}

// SetTransientState sets transient storage for a given account. It
// adds the change to the journal so that it can be rolled back
// to its previous value if there is a revert. (for more see https://eips.ethereum.org/EIPS/eip-6780)
func (s *StateDB) SetTransientState(addr common.Address, key, value common.Hash) {
	prev := s.GetTransientState(addr, key)
	if prev == value {
		return
	}
	s.journal.append(transientStorageChange{
		account:  &addr,
		key:      key,
		prevalue: prev,
	})
	s.transientStorage.Set(addr, key, value)
}

func (s *StateDB) SelfDestruct(address common.Address) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) types.UnitData {
		so.Suicided = true
		so.Account.Balance = uint256.NewInt(0)
		s.suicides = append(s.suicides, address)
		return so
	})
}

func (s *StateDB) HasSelfDestructed(address common.Address) bool {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return false
	}
	return stateObject.Suicided
}

// Selfdestruct6780 - EIP-6780 changes the functionality of the SELFDESTRUCT opcode.
// The new functionality will be only to send all Ether in the account to the caller,
// except that the current behaviour is preserved when SELFDESTRUCT is called in the same transaction
// a contract was created.
func (s *StateDB) Selfdestruct6780(address common.Address) {
	if _, ok := s.created[address]; ok {
		s.SelfDestruct(address)
	}
}

func (s *StateDB) Exist(address common.Address) bool {
	so := s.getStateObject(address.Bytes(), false)
	return so != nil
}

func (s *StateDB) Empty(address common.Address) bool {
	so := s.getStateObject(address.Bytes(), false)
	return so == nil || so.empty()
}

// Prepare handles the preparatory steps for executing a state transition with.
// This method must be invoked before state transition.
//
// Berlin fork:
// - Add sender to access list (2929)
// - Add destination to access list (2929)
// - Add precompiles to access list (2929)
// - Add the contents of the optional tx access list (2930)
//
// Potential EIPs:
// - Reset access list (Berlin)
// - Add coinbase to access list (EIP-3651)
// - Reset transient storage (EIP-1153)
func (s *StateDB) Prepare(rules params.Rules, sender, coinbase common.Address, dest *common.Address, precompiles []common.Address, txAccesses ethtypes.AccessList) {
	if rules.IsBerlin {
		s.AddAddressToAccessList(sender)
		if dest != nil {
			s.AddAddressToAccessList(*dest)
			// If it's a create-tx, the destination will be added inside evm.create
		}
		for _, addr := range precompiles {
			s.AddAddressToAccessList(addr)
		}
		for _, el := range txAccesses {
			s.AddAddressToAccessList(el.Address)
			for _, key := range el.StorageKeys {
				s.AddSlotToAccessList(el.Address, key)
			}
		}
		// Currently ignore rules.IsShanghai and do not add coninbase address to access list, there is no coinbase for AB
		/*
			if rules.IsShanghai { // EIP-3651: warm coinbase
				s.AddAddressToAccessList(coinbase)
			}
		*/
	}
	s.transientStorage = newTransientStorage()
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
			s.journal.revert(s, rev.journalIdx)
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
	s.logs = append(s.logs, &evm.LogEntry{
		Address: log.Address,
		Topics:  log.Topics,
		Data:    log.Data,
	})
}

func (s *StateDB) GetLogs() []*evm.LogEntry {
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
		if so.Suicided {
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
	s.created = map[common.Address]struct{}{}
	return nil
}

func (s *StateDB) SetAlphaBillData(address common.Address, fee *AlphaBillLink) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID, false)
	if stateObject == nil {
		return
	}
	s.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("Setting fee credit data for an account: %v", address))
	s.errDB = s.executeUpdate(unitID, func(so *StateObject) types.UnitData {
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

func (s *StateDB) executeUpdate(id types.UnitID, updateFunc func(so *StateObject) types.UnitData) error {
	if err := s.tree.Apply(state.UpdateUnitData(id, func(data types.UnitData) (types.UnitData, error) {
		so, ok := data.(*StateObject)
		if !ok {
			return so, fmt.Errorf("unit data is not an instance of StateObject")
		}
		return updateFunc(so), nil
	})); err != nil {
		return err
	}
	addr := common.BytesToAddress(id)
	s.journal.append(accountChange{account: &addr})
	return nil
}
