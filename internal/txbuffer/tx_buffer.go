package txbuffer

import (
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var (
	ErrTxBufferFull   = errors.New("tx buffer is full")
	ErrInvalidMaxSize = errors.New("invalid maximum size")
	ErrTxIsNil        = errors.New("tx is nil")
	ErrTxInBuffer     = errors.New("tx already in tx buffer")
	ErrTxNotFound     = errors.New("tx not found")
)

type (
	Transaction interface {
		IDHash() string
	}

	// TxBuffer is an in-memory data structure containing the set of unconfirmed transactions.
	TxBuffer struct {
		mutex        sync.Mutex               // mutex for locks
		transactions map[string][]Transaction // map containing valid pending transactions.
		count        uint32                   // number of transactions in the tx-buffer
		maxSize      uint32                   // maximum TxBuffer size.
	}
)

// New creates a new instance of the TxBuffer. MaxSize specifies the total number of transactions the TxBuffer may
// contain.
func New(maxSize uint32) (*TxBuffer, error) {
	if maxSize < 1 {
		return nil, ErrInvalidMaxSize
	}
	return &TxBuffer{
		maxSize:      maxSize,
		transactions: make(map[string][]Transaction),
	}, nil
}

// Add adds the given transaction to the transaction buffer. Returns an error if the transaction isn't valid, is
// already present in the TxBuffer, or TxBuffer is full.
func (t *TxBuffer) Add(tx Transaction) error {
	if tx == nil {
		return ErrTxIsNil
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.count >= t.maxSize {
		return ErrTxBufferFull
	}

	txId := tx.IDHash()
	_, found := t.transactions[txId]
	if found {
		return ErrTxInBuffer
	}
	t.transactions[txId] = append(t.transactions[txId], tx)
	t.count++
	return nil
}

// GetAll returns all transactions from the TxBuffer. All returned transactions are removed from the TxBuffer.
func (t *TxBuffer) GetAll() []Transaction {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	values := make([]Transaction, t.count)

	var i uint32 = 0
	for _, v := range t.transactions {
		for _, tr := range v {
			values[i] = tr
			i++
		}
	}
	t.transactions = make(map[string][]Transaction)
	t.count = 0
	return values
}

// Remove removes the transaction with given domain.TxID from the TxBuffer.
func (t *TxBuffer) Remove(id string) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	values, found := t.transactions[id]
	if !found {
		return ErrTxNotFound
	}
	delete(t.transactions, id)
	t.count -= uint32(len(values))
	return nil
}

// Count returns the total number of transactions in the TxBuffer.
func (t *TxBuffer) Count() uint32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.count
}
