package statedb

import (
	"errors"
	"math/big"

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

var _ vm.StateDB = &StateDB{}
var log = logger.CreateForPackage()

type StateDB struct {
	tree       *state.State
	errDB      error
	accessList *accessList
}

func NewStateDB(tree *state.State) *StateDB {
	return &StateDB{
		tree:       tree,
		accessList: newAccessList(),
	}
}

func (s *StateDB) CreateAccount(address common.Address) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject == nil {
		log.Trace("Adding an account: %v", address)
		s.errDB = s.tree.Apply(state.AddUnit(
			unitID,
			script.PredicateAlwaysFalse(),
			&StateObject{Address: address, Account: &Account{Nonce: 0, Balance: big.NewInt(0), CodeHash: emptyCodeHash}, Storage: map[common.Hash]common.Hash{}},
		))
	}
}

func (s *StateDB) SubBalance(address common.Address, amount *big.Int) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		s.errDB = s.tree.Apply(state.UpdateUnitData(
			unitID, func(data state.UnitData) (state.UnitData, error) {
				if amount.Sign() == 0 {
					return data, nil
				}
				newBalance := new(big.Int).Sub(data.(*StateObject).Account.Balance, amount)
				// TODO return an error id data is not a StateObject
				data.(*StateObject).Account.Balance = newBalance
				return data, nil
			},
		))
	}
}

func (s *StateDB) AddBalance(address common.Address, amount *big.Int) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		s.errDB = s.tree.Apply(state.UpdateUnitData(
			unitID, func(data state.UnitData) (state.UnitData, error) {
				if amount.Sign() == 0 {
					return data, nil
				}
				newBalance := new(big.Int).Add(data.(*StateObject).Account.Balance, amount)
				// TODO return an error id data is not a StateObject
				data.(*StateObject).Account.Balance = newBalance
				return data, nil
			},
		))
	}
}

func (s *StateDB) GetBalance(address common.Address) *big.Int {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		return stateObject.Account.Balance
	}
	return big.NewInt(0)
}

func (s *StateDB) GetNonce(address common.Address) uint64 {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		return stateObject.Account.Nonce
	}
	return 0
}

func (s *StateDB) SetNonce(address common.Address, nonce uint64) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		log.Trace("Setting a new nonce %v for an account: %v", nonce, address)
		s.errDB = s.tree.Apply(state.UpdateUnitData(
			unitID, func(data state.UnitData) (state.UnitData, error) {
				// TODO return an error id data is not a StateObject
				data.(*StateObject).Account.Nonce = nonce
				return data, nil
			},
		))
	}
}

func (s *StateDB) GetCodeHash(address common.Address) common.Hash {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		return common.BytesToHash(stateObject.Account.CodeHash)
	}
	return common.Hash{}
}

func (s *StateDB) GetCode(address common.Address) []byte {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		return stateObject.Account.Code
	}
	return nil
}

func (s *StateDB) SetCode(address common.Address, code []byte) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		log.Trace("Setting code %X for an account: %v", code, address)
		s.errDB = s.tree.Apply(state.UpdateUnitData(
			unitID, func(data state.UnitData) (state.UnitData, error) {
				// TODO return an error id data is not a StateObject
				data.(*StateObject).Account.Code = code
				data.(*StateObject).Account.CodeHash = crypto.Keccak256Hash(code).Bytes()
				return data, nil
			},
		))
	}
}

func (s *StateDB) GetCodeSize(address common.Address) int {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil {
		return len(stateObject.Account.Code)
	}
	return 0
}

func (s *StateDB) AddRefund(gas uint64) {
	// TODO implement
}

func (s *StateDB) SubRefund(gas uint64) {
	// TODO implement
}

func (s *StateDB) GetRefund() uint64 {
	// TODO implement
	return 0
}

func (s *StateDB) GetCommittedState(address common.Address, key common.Hash) common.Hash {
	// TODO after integrating a new AVLTree and stateTree this code must use AVLTree snapshots
	stateObject := s.getStateObject(address.Bytes())
	if stateObject == nil {
		return common.Hash{}
	}
	return stateObject.Storage[key]
}

func (s *StateDB) GetState(address common.Address, key common.Hash) common.Hash {
	// TODO after integrating a new AVLTree and stateTree this code must use AVLTree snapshots
	stateObject := s.getStateObject(address.Bytes())
	if stateObject == nil {
		return common.Hash{}
	}
	return stateObject.Storage[key]
}

func (s *StateDB) SetState(address common.Address, key common.Hash, value common.Hash) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject == nil {
		return
	}
	log.Trace("Setting a state (key=%v, value=%v) for an account: %v", key, value, address)
	s.errDB = s.tree.Apply(state.UpdateUnitData(
		unitID, func(data state.UnitData) (state.UnitData, error) {
			// TODO return an error id data is not a StateObject
			data.(*StateObject).Storage[key] = value
			return data, nil
		},
	))
}

func (s *StateDB) Suicide(address common.Address) bool {
	// TODO implement
	return false
}

func (s *StateDB) HasSuicided(address common.Address) bool {
	// TODO implement
	return false
}

func (s *StateDB) Exist(address common.Address) bool {
	so := s.getStateObject(address.Bytes())
	return so != nil
}

func (s *StateDB) Empty(address common.Address) bool {
	so := s.getStateObject(address.Bytes())
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
	//TODO implement after integrating a new AVLTree.
}

func (s *StateDB) Snapshot() int {
	//TODO implement after integrating a new AVLTree.
	return 0
}

func (s *StateDB) AddLog(log *ethtypes.Log) {
	//TODO implement me
	panic("implement me")
}

func (s *StateDB) AddPreimage(_ common.Hash, _ []byte) {
	// no-op! the EnablePreimageRecording flag is disabled in vm.Config
}

func (s *StateDB) ForEachStorage(address common.Address, f func(common.Hash, common.Hash) bool) error {
	panic("implement ForEachStorage")
}

func (s *StateDB) SetAlphaBillData(address common.Address, fee *AlphaBillLink) {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject == nil {
		return
	}
	log.Trace("Setting fee data for account: %v", address)
	s.errDB = s.tree.Apply(state.UpdateUnitData(
		unitID, func(data state.UnitData) (state.UnitData, error) {
			// TODO return an error id data is not a StateObject
			data.(*StateObject).AlphaBill = fee
			return data, nil
		},
	))
}

func (s *StateDB) GetAlphaBillData(address common.Address) *AlphaBillLink {
	unitID := address.Bytes()
	stateObject := s.getStateObject(unitID)
	if stateObject != nil && stateObject.AlphaBill != nil {
		return stateObject.AlphaBill
	}
	return nil
}

func (s *StateDB) getStateObject(unitID types.UnitID) *StateObject {
	// TODO
	u, err := s.tree.GetUnit(unitID, false)
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
