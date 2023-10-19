package unit

import (
	"bytes"
	"hash"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/util"
)

// FeeCreditRecord state tree unit data of fee credit records.
// Holds fee credit balance for individual users,
// not to be confused with fee credit bills which contain aggregate fees for a given partition.
type FeeCreditRecord struct {
	Balance  uint64 // current balance
	Backlink []byte // hash of the last “addFC”, "closeFC", "lockFC" or "unlockFC" transaction
	Timeout  uint64 // the earliest round number when this record may be “garbage collected” if the balance goes to zero
	Locked   uint64 // locked status of the fee credit record, non-zero value means locked
}

func (b *FeeCreditRecord) Write(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(b.Balance))
	hasher.Write(b.Backlink)
	hasher.Write(util.Uint64ToBytes(b.Timeout))
	hasher.Write(util.Uint64ToBytes(b.Locked))
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

func (b *FeeCreditRecord) GetBacklink() []byte {
	if b == nil {
		return nil
	}
	return b.Backlink
}

func (b *FeeCreditRecord) IsLocked() bool {
	return b.Locked != 0
}
