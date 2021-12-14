package txpool

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"sync"
)

var (
	ErrTxPoolFull        = errors.New("tx pool is full")
	ErrInvalidMaxSize    = errors.New("invalid tx pool maximum size")
	ErrTxIsNil           = errors.New("tx is nil")
	ErrTxInPool          = errors.New("tx already in tx pool")
)
// TODO AB-33
type (

	// TxPool is an in-memory data structure containing the set of unconfirmed transactions.
	TxPool struct {
		mutext       sync.Mutex                           // mutex for locks
		transactions map[domain.TxID]*domain.PaymentOrder // map containing valid pending transactions.
		maxSize      uint32                               // maximum TxPool size.
	}
)

// New creates a new instance of the txPool.
func New(maxSize uint32) (*TxPool, error) {
	if maxSize < 1 {
		return nil, ErrInvalidMaxSize
	}
	return &TxPool{
		maxSize:      maxSize,
		transactions: make(map[domain.TxID]*domain.PaymentOrder),
	}, nil
}

// Add adds the given transaction to the transaction buffer. Returns an error if the transaction isn't valid or is
// already present in the txPool.
func (t *TxPool) Add(tx *domain.PaymentOrder) error {
	if tx == nil {
		return ErrTxIsNil
	}
	t.mutext.Lock()
	defer t.mutext.Unlock()

	if t.size() >= t.maxSize {
		return ErrTxPoolFull
	}

	txId := tx.ID()
	_, found := t.transactions[txId]
	if found {
		return ErrTxInPool
	}
	t.transactions[txId] = tx
	return nil
}

func (t *TxPool) Count() uint32 {
	t.mutext.Lock()
	defer t.mutext.Unlock()
	return t.size()
}

func (t *TxPool) size() uint32 {
	return uint32(len(t.transactions))
}
