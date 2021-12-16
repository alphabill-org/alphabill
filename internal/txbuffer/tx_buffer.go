package txbuffer

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"sync"
)

var (
	ErrTxBufferFull   = errors.New("tx buffer is full")
	ErrInvalidMaxSize = errors.New("invalid tx buffer maximum size")
	ErrTxIsNil        = errors.New("tx is nil")
	ErrTxInBuffer     = errors.New("tx already in tx buffer")
	ErrTxNotFound     = errors.New("tx not found")
)

type (

	// TxBuffer is an in-memory data structure containing the set of unconfirmed transactions.
	TxBuffer struct {
		mutex        sync.Mutex                           // mutex for locks
		transactions map[domain.TxID]*domain.PaymentOrder // map containing valid pending transactions.
		maxSize      uint32                               // maximum TxBuffer size.
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
		transactions: make(map[domain.TxID]*domain.PaymentOrder),
	}, nil
}

// Add adds the given transaction to the transaction buffer. Returns an error if the transaction isn't valid, is
// already present in the TxBuffer, or TxBuffer is full.
func (t *TxBuffer) Add(tx *domain.PaymentOrder) error {
	if tx == nil {
		return ErrTxIsNil
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.size() >= t.maxSize {
		return ErrTxBufferFull
	}

	txId := tx.ID()
	_, found := t.transactions[txId]
	if found {
		return ErrTxInBuffer
	}
	t.transactions[txId] = tx
	return nil
}

// GetAll returns all transactions from the TxBuffer. All returned transactions are removed from the TxBuffer.
func (t *TxBuffer) GetAll() []*domain.PaymentOrder {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	values := make([]*domain.PaymentOrder, 0)

	for k, v := range t.transactions {
		values = append(values, v)
		delete(t.transactions, k)
	}
	return values
}

// Remove removes the transaction with given domain.TxID from the TxBuffer.
func (t *TxBuffer) Remove(id domain.TxID) error {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	_, found := t.transactions[id]
	if !found {
		return ErrTxNotFound
	}
	delete(t.transactions, id)
	return nil
}

// Count returns the total number of transactions in the TxBuffer.
func (t *TxBuffer) Count() uint32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.size()
}

func (t *TxBuffer) size() uint32 {
	return uint32(len(t.transactions))
}
