package txbuffer

import (
	"context"
	gocrypto "crypto"
	"math/rand"
	"reflect"
	"sync"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

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

	// TxBuffer is an in-memory data structure containing the set of unconfirmed transactions.
	TxBuffer struct {
		mutex         sync.Mutex                          // mutex for locks
		transactions  map[string]*transaction.Transaction // map containing valid pending transactions.
		count         uint32                              // number of transactions in the tx-buffer
		maxSize       uint32                              // maximum TxBuffer size.
		hashAlgorithm gocrypto.Hash
	}

	TxHandler func(tx *transaction.Transaction) bool
)

// New creates a new instance of the TxBuffer. MaxSize specifies the total number of transactions the TxBuffer may
// contain.
func New(maxSize uint32, hashAlgorithm gocrypto.Hash) (*TxBuffer, error) {
	if maxSize < 1 {
		return nil, ErrInvalidMaxSize
	}
	return &TxBuffer{
		maxSize:       maxSize,
		hashAlgorithm: hashAlgorithm,
		transactions:  make(map[string]*transaction.Transaction),
	}, nil
}

// Add adds the given transaction to the transaction buffer. Returns an error if the transaction isn't valid, is
// already present in the TxBuffer, or TxBuffer is full.
func (t *TxBuffer) Add(tx *transaction.Transaction) error {
	if tx == nil {
		return ErrTxIsNil
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.count >= t.maxSize {
		return ErrTxBufferFull
	}

	txHash, err := tx.Hash(t.hashAlgorithm)
	if err != nil {
		return err
	}
	txId := string(txHash)
	_, found := t.transactions[txId]
	if found {
		return ErrTxInBuffer
	}
	t.transactions[txId] = tx
	t.count++
	return nil
}

func (t *TxBuffer) Process(ctx context.Context, wg *sync.WaitGroup, process TxHandler) {
	for {
		select {
		case <-ctx.Done():
			wg.Done()
			return
		default:
			if id, tx := t.getNext(); tx != nil {
				if !process(tx) {
					continue
				}
				t.Remove(id)
			}
		}

	}
}

// GetAll returns all transactions from the TxBuffer. All returned transactions are removed from the TxBuffer.
func (t *TxBuffer) GetAll() []*transaction.Transaction {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	values := make([]*transaction.Transaction, t.count)

	var i uint32 = 0
	for _, v := range t.transactions {
		values[i] = v
		i++
	}
	t.transactions = make(map[string]*transaction.Transaction)
	t.count = 0
	return values
}

// Remove removes the transaction with given id from the TxBuffer.
func (t *TxBuffer) Remove(id string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	_, found := t.transactions[id]
	if found {
		delete(t.transactions, id)
		t.count--
	}
}

// Count returns the total number of transactions in the TxBuffer.
func (t *TxBuffer) Count() uint32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.count
}

// getNext returns a transaction from the buffer
func (t *TxBuffer) getNext() (string, *transaction.Transaction) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	if t.count == 0 {
		return "", nil
	}
	keys := reflect.ValueOf(t.transactions).MapKeys()
	// #nosec G404
	randomKey := keys[rand.Intn(len(keys))]
	key := randomKey.Interface().(string)
	return key, t.transactions[key]
}
