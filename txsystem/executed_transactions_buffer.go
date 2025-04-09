package txsystem

import (
	"crypto"
	"fmt"
	"sort"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
)

// ETBuffer Executed Transactions Buffer. It is used to avoid processing the same transaction multiple times which
// would allow someone else to drain your fee credit by re-broadcasting the already processed transaction, since fees
// would still be charged for the duplicate transaction for verifying the fee predicate.
type ETBuffer struct {
	executedTransactions map[string]uint64
	pendingTransactions  map[string]uint64 // keep current round transactions in separate map in order to support Commit and Revert functions.
}

type ETBufferOption func(*ETBuffer)

func WithExecutedTxs(txs map[string]uint64) ETBufferOption {
	return func(etb *ETBuffer) {
		etb.executedTransactions = txs
	}
}

func NewETBuffer(opts ...ETBufferOption) *ETBuffer {
	etb := &ETBuffer{}
	for _, opt := range opts {
		opt(etb)
	}
	if etb.executedTransactions == nil {
		etb.executedTransactions = make(map[string]uint64)
	}
	if etb.pendingTransactions == nil {
		etb.pendingTransactions = make(map[string]uint64)
	}
	return etb
}

func (b *ETBuffer) Add(txID string, timeout uint64) {
	b.pendingTransactions[txID] = timeout
}

func (b *ETBuffer) Get(txID string) (uint64, bool) {
	timeout, f := b.executedTransactions[txID]
	if f {
		return timeout, true
	}
	timeout, f = b.pendingTransactions[txID]
	if f {
		return timeout, true
	}
	return 0, false
}

// Commit commits pending changes.
func (b *ETBuffer) Commit() {
	for txID, timeout := range b.pendingTransactions {
		b.executedTransactions[txID] = timeout
	}
	b.pendingTransactions = make(map[string]uint64)
}

// Revert reverts pending changes.
func (b *ETBuffer) Revert() {
	b.pendingTransactions = make(map[string]uint64)
}

// ClearExpired deletes expired transactions from the buffer.
func (b *ETBuffer) ClearExpired(currentRound uint64) {
	for txHash, deletionRound := range b.executedTransactions {
		if deletionRound <= currentRound {
			delete(b.executedTransactions, txHash)
		}
	}
	for txHash, deletionRound := range b.pendingTransactions {
		if deletionRound <= currentRound {
			delete(b.pendingTransactions, txHash)
		}
	}
}

// Hash calculates hash of the executed transactions buffer.
func (b *ETBuffer) Hash() ([]byte, error) {
	// sort transactions by their hash values
	txHashes := make([]string, 0, len(b.executedTransactions)+len(b.pendingTransactions))
	for txHash := range b.executedTransactions {
		txHashes = append(txHashes, txHash)
	}
	for txHash := range b.pendingTransactions {
		txHashes = append(txHashes, txHash)
	}
	sort.Strings(txHashes)

	hasher := abhash.New(crypto.SHA256.New())
	for _, txHash := range txHashes {
		txTimeout, f := b.Get(txHash)
		if !f {
			// sanity check, should never happen
			return nil, fmt.Errorf("transaction not found for hash %x", txHash)
		}
		hasher.Write(txHash)
		hasher.Write(txTimeout)
	}
	return hasher.Sum()
}
