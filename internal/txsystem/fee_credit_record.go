package txsystem

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/util"
)

// FeeCreditRecord state tree unit data of fee credit records.
// Holds fee credit balance for individual users,
// not to be confused with fee credit bills which contain aggregate fees for a given partition.
type FeeCreditRecord struct {
	balance int64  // current balance
	hash    []byte // hash of the last “add fee credit” transaction that incremented the balance of this record
	timeout uint64 // the earliest round number when this record may be “garbage collected” if the balance goes to zero
}

func (b *FeeCreditRecord) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(b.balance)))
	hasher.Write(b.hash)
	hasher.Write(util.Uint64ToBytes(b.timeout))
}

func (b *FeeCreditRecord) Value() rma.SummaryValue {
	// Fee Credit Record value is not included in money invariant.
	return rma.Uint64SummaryValue(0)
}
