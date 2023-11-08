package unitlock

import (
	"crypto"
	"errors"
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/types"
)

// the same constants are used in server side locking
const (
	LockReasonAddFees = 1 + iota
	LockReasonReclaimFees
	LockReasonCollectDust
)

const (
	UnitStoreDBFileName = "unitlock.db"
)

type (
	LockReason uint

	UnitLocker struct {
		db UnitStore
	}

	LockedUnit struct {
		AccountID    []byte         `json:"accountId"`    // account id of the unit e.g. a public key
		UnitID       []byte         `json:"unitId"`       // id of the locked unit
		TxHash       []byte         `json:"txHash"`       // state hash of the locked unit
		SystemID     []byte         `json:"systemId"`     // target system id of the locked unit
		LockReason   LockReason     `json:"lockReason"`   // reason for locking the bill
		Transactions []*Transaction `json:"transactions"` // transactions that must be confirmed/failed in order to unlock the bill
	}

	Transaction struct {
		TxOrder *types.TransactionOrder `json:"txOrder"`
		TxHash  []byte                  `json:"txHash"`
	}

	UnitStore interface {
		GetUnit(accountID, unitID []byte) (*LockedUnit, error)
		GetUnits(accountID []byte) ([]*LockedUnit, error)
		PutUnit(unit *LockedUnit) error
		DeleteUnit(accountID, unitID []byte) error
		Close() error
	}
)

func NewUnitLocker(dir string) (*UnitLocker, error) {
	store, err := NewBoltStore(filepath.Join(dir, UnitStoreDBFileName))
	if err != nil {
		return nil, err
	}
	return &UnitLocker{db: store}, nil
}

func NewLockedUnit(accountID, unitID, txHash, systemID []byte, lockReason LockReason, transactions ...*Transaction) *LockedUnit {
	return &LockedUnit{
		AccountID:    accountID,
		UnitID:       unitID,
		TxHash:       txHash,
		SystemID:     systemID,
		LockReason:   lockReason,
		Transactions: transactions,
	}
}

func NewTransaction(txo *types.TransactionOrder) *Transaction {
	return &Transaction{
		TxOrder: txo,
		TxHash:  txo.Hash(crypto.SHA256),
	}
}

func (l *UnitLocker) LockUnit(unit *LockedUnit) error {
	return l.db.PutUnit(unit)
}

func (l *UnitLocker) UnlockUnit(accountID, unitID []byte) error {
	return l.db.DeleteUnit(accountID, unitID)
}

func (l *UnitLocker) GetUnit(accountID, unitID []byte) (*LockedUnit, error) {
	return l.db.GetUnit(accountID, unitID)
}

func (l *UnitLocker) GetUnits(accountID []byte) ([]*LockedUnit, error) {
	return l.db.GetUnits(accountID)
}

func (l *UnitLocker) Close() error {
	return l.db.Close()
}

func (r LockReason) String() string {
	switch r {
	case LockReasonAddFees:
		return "locked for adding fees"
	case LockReasonReclaimFees:
		return "locked for reclaiming fees"
	case LockReasonCollectDust:
		return "locked for dust collection"
	}
	return ""
}

type InMemoryUnitLocker struct {
	units map[string]map[string]*LockedUnit
}

func NewInMemoryUnitLocker() *InMemoryUnitLocker {
	return &InMemoryUnitLocker{units: map[string]map[string]*LockedUnit{}}
}

func (m *InMemoryUnitLocker) GetUnits(accountID []byte) ([]*LockedUnit, error) {
	var units []*LockedUnit
	for _, unit := range m.units[string(accountID)] {
		units = append(units, unit)
	}
	return units, nil
}

func (m *InMemoryUnitLocker) GetUnit(accountID, unitID []byte) (*LockedUnit, error) {
	unitsMap := m.units[string(accountID)]
	return unitsMap[string(unitID)], nil
}

func (m *InMemoryUnitLocker) LockUnit(lockedBill *LockedUnit) error {
	unitsMap, ok := m.units[string(lockedBill.AccountID)]
	if !ok {
		unitsMap = map[string]*LockedUnit{}
		m.units[string(lockedBill.AccountID)] = unitsMap
	}
	unitsMap[string(lockedBill.UnitID)] = lockedBill
	return nil
}

func (m *InMemoryUnitLocker) UnlockUnit(accountID, unitID []byte) error {
	unitsMap, ok := m.units[string(accountID)]
	if !ok {
		return nil
	}
	delete(unitsMap, string(unitID))
	return nil
}

func (m *InMemoryUnitLocker) Close() error {
	return nil
}

func (l *LockedUnit) isValid() error {
	if l == nil {
		return errors.New("unit is nil")
	}
	if l.AccountID == nil {
		return errors.New("unit account id is nil")
	}
	if l.UnitID == nil {
		return errors.New("unit id is nil")
	}
	if l.SystemID == nil {
		return errors.New("system id is nil")
	}
	if l.TxHash == nil {
		return errors.New("tx hash is nil")
	}
	return nil
}
