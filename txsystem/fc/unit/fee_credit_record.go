package unit

import (
	"bytes"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/state"
	"github.com/fxamacker/cbor/v2"
)

const UNLOCKED uint64 = 0

type FeeCreditType interface {
	// AddCredit - adds v to balance, sets Backlink equal to copy of b, Tiemout to t and lock to 0 (unlocks)
	AddCredit(v uint64, b []byte, t uint64)
	// DecCredit - decrements balance by v and updates backlink if backlink is not nil (probably should be different methods)
	DecCredit(v uint64, b []byte)
	GetBalance() uint64
	GetTimeout() uint64
	GetBacklink() []byte
	// UpdateLock - sets lock value to lockstatus, decrements balace by fee and updates backlink
	UpdateLock(lockStatus uint64, fee uint64, backlink []byte)
	// IsLocked - returns true if lock status is not UNLOCKED
	IsLocked() bool
}

type GenericFeeCreditRecord interface {
	FeeCreditType
	state.UnitData
}

// FeeCreditRecord state tree unit data of fee credit records.
// Holds fee credit balance for individual users,
// not to be confused with fee credit bills which contain aggregate fees for a given partition.
type FeeCreditRecord struct {
	_        struct{} `cbor:",toarray"`
	Balance  uint64   // current balance
	Backlink []byte   // hash of the last “addFC”, "closeFC", "lockFC" or "unlockFC" transaction
	Timeout  uint64   // the earliest round number when this record may be “garbage collected” if the balance goes to zero
	Locked   uint64   // locked status of the fee credit record, non-zero value means locked
}

func (b *FeeCreditRecord) Write(hasher hash.Hash) error {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(b)
	if err != nil {
		return fmt.Errorf("fee credit serialization error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (b *FeeCreditRecord) SummaryValueInput() uint64 {
	return 0
}

func (b *FeeCreditRecord) Copy() state.UnitData {
	return &FeeCreditRecord{
		Balance:  b.Balance,
		Backlink: bytes.Clone(b.Backlink),
		Timeout:  b.Timeout,
		Locked:   b.Locked,
	}
}

func (b *FeeCreditRecord) AddCredit(v uint64, backlink []byte, timeout uint64) {
	b.Balance += v
	b.Backlink = bytes.Clone(backlink)
	b.Timeout = timeout
	b.Locked = UNLOCKED
}

func (b *FeeCreditRecord) DecCredit(v uint64, backlink []byte) {
	b.Balance -= v
	if backlink != nil {
		b.Backlink = bytes.Clone(backlink)
	}
}

func (b *FeeCreditRecord) GetBalance() uint64 {
	return b.Balance
}

func (b *FeeCreditRecord) GetTimeout() uint64 {
	return b.Timeout
}

func (b *FeeCreditRecord) GetBacklink() []byte {
	if b == nil {
		return nil
	}
	return b.Backlink
}

func (b *FeeCreditRecord) UpdateLock(lock uint64, fee uint64, backlink []byte) {
	b.Locked = lock
	b.Balance -= fee
	b.Backlink = bytes.Clone(backlink)
}

func (b *FeeCreditRecord) IsLocked() bool {
	return b.Locked != UNLOCKED
}
