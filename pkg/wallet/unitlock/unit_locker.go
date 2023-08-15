package unitlock

import (
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	ReasonAddFees LockReason = iota
	ReasonReclaimFees
	ReasonCollectDust
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
		UnitID       []byte         `json:"unitId"`       // if of the locked unit
		TxHash       []byte         `json:"txHash"`       // state hash of the locked unit
		LockReason   LockReason     `json:"lockReason"`   // reason for locking the bill
		Transactions []*Transaction `json:"transactions"` // transactions that must be confirmed/failed in order to unlock the bill
	}

	Transaction struct {
		TxOrder     *types.TransactionOrder `json:"txOrder"`
		PayloadType string                  `json:"payloadType"`
		Timeout     uint64                  `json:"timeout"`
		TxHash      []byte                  `json:"txHash"`
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

func NewTransaction(txo *types.TransactionOrder, txHash []byte) *Transaction {
	return &Transaction{
		TxOrder:     txo,
		PayloadType: txo.PayloadType(),
		Timeout:     txo.Timeout(),
		TxHash:      txHash,
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
	case ReasonAddFees:
		return "locked for adding fees"
	case ReasonReclaimFees:
		return "locked for reclaiming fees"
	case ReasonCollectDust:
		return "locked for dust collection"
	}
	return ""
}
