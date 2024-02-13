package unit

import (
	"bytes"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/types"
	"github.com/fxamacker/cbor/v2"
)

// FeeCreditRecord state tree unit data of fee credit records.
// Holds fee credit balance for individual users,
// not to be confused with fee credit bills which contain aggregate fees for a given partition.
type FeeCreditRecord struct {
	_        struct{}  `cbor:",toarray"`
	Balance  types.Fee // current balance
	Backlink []byte    // hash of the last “addFC”, "closeFC", "lockFC" or "unlockFC" transaction
	Timeout  uint64    // the earliest round number when this record may be “garbage collected” if the balance goes to zero
	Locked   uint64    // locked status of the fee credit record, non-zero value means locked
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

func (b *FeeCreditRecord) GetBacklink() []byte {
	if b == nil {
		return nil
	}
	return b.Backlink
}

func (b *FeeCreditRecord) IsLocked() bool {
	return b.Locked != 0
}
