package unitlock

import (
	"crypto"
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	LockReasonAddFees LockReason = iota
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
		UnitID       []byte         `json:"unitId"`       // id of the locked unit
		TxHash       []byte         `json:"txHash"`       // state hash of the locked unit
		LockReason   LockReason     `json:"lockReason"`   // reason for locking the bill
		Transactions []*Transaction `json:"transactions"` // transactions that must be confirmed/failed in order to unlock the bill
	}

	Transaction struct {
		TxOrder *types.TransactionOrder `json:"txOrder"`
		TxHash  []byte                  `json:"txHash"`
	}

	UnitStore interface {
		GetUnit(unitID []byte) (*LockedUnit, error)
		GetUnits() ([]*LockedUnit, error)
		PutUnit(unit *LockedUnit) error
		DeleteUnit(unitID []byte) error
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

func NewLockedUnit(unitID []byte, txHash []byte, lockReason LockReason, transactions ...*Transaction) *LockedUnit {
	return &LockedUnit{
		UnitID:       unitID,
		TxHash:       txHash,
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

func (l *UnitLocker) UnlockUnit(unitID []byte) error {
	return l.db.DeleteUnit(unitID)
}

func (l *UnitLocker) GetUnit(unitID []byte) (*LockedUnit, error) {
	return l.db.GetUnit(unitID)
}

func (l *UnitLocker) GetUnits() ([]*LockedUnit, error) {
	return l.db.GetUnits()
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
	units map[string]*LockedUnit
}

func NewInMemoryUnitLocker() *InMemoryUnitLocker {
	return &InMemoryUnitLocker{units: map[string]*LockedUnit{}}
}

func (m *InMemoryUnitLocker) GetUnits() ([]*LockedUnit, error) {
	var units []*LockedUnit
	for _, unit := range m.units {
		units = append(units, unit)
	}
	return units, nil
}

func (m *InMemoryUnitLocker) GetUnit(unitID []byte) (*LockedUnit, error) {
	return m.units[string(unitID)], nil
}

func (m *InMemoryUnitLocker) LockUnit(lockedBill *LockedUnit) error {
	m.units[string(lockedBill.UnitID)] = lockedBill
	return nil
}

func (m *InMemoryUnitLocker) UnlockUnit(unitID []byte) error {
	delete(m.units, string(unitID))
	return nil
}

func (m *InMemoryUnitLocker) Close() error {
	return nil
}
