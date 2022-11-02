package txbuffer

import (
	"context"
	gocrypto "crypto"
	"sync"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var (
	ErrTxBufferFull   = errors.New("tx buffer is full")
	ErrInvalidMaxSize = errors.New("invalid maximum size")
	ErrTxIsNil        = errors.New("tx is nil")
	ErrTxInBuffer     = errors.New("tx already in tx buffer")

	transactionsCounter          = metrics.GetOrRegisterCounter("transaction/buffer/size")
	transactionsRejectedCounter  = metrics.GetOrRegisterCounter("transaction/buffer/rejected/full")
	transactionsDuplicateCounter = metrics.GetOrRegisterCounter("transaction/buffer/rejected/duplicate")
)

type (

	// TxBuffer is an in-memory data structure containing the set of unconfirmed transactions.
	TxBuffer struct {
		mutex          sync.Mutex                             // mutex for locks
		transactions   map[string]txsystem.GenericTransaction // map containing valid pending transactions.
		transactionsCh chan txsystem.GenericTransaction
		count          uint32 // number of transactions in the tx-buffer
		maxSize        uint32 // maximum TxBuffer size.
		hashAlgorithm  gocrypto.Hash
	}

	// TxHandler handles the transaction. Must return true if the transaction must be removed from the transaction
	// buffer.
	TxHandler func(tx txsystem.GenericTransaction) bool
)

// New creates a new instance of the TxBuffer. MaxSize specifies the total number of transactions the TxBuffer may
// contain.
func New(maxSize uint32, hashAlgorithm gocrypto.Hash) (*TxBuffer, error) {
	if maxSize < 1 {
		return nil, ErrInvalidMaxSize
	}
	return &TxBuffer{
		maxSize:        maxSize,
		hashAlgorithm:  hashAlgorithm,
		transactions:   make(map[string]txsystem.GenericTransaction),
		transactionsCh: make(chan txsystem.GenericTransaction, maxSize),
	}, nil
}

func (t *TxBuffer) Close() {
	close(t.transactionsCh)
}

func (t *TxBuffer) Capacity() uint32 {
	return t.maxSize
}

// Add adds the given transaction to the transaction buffer. Returns an error if the transaction isn't valid, is
// already present in the TxBuffer, or TxBuffer is full.
func (t *TxBuffer) Add(tx txsystem.GenericTransaction) error {
	logger.Debug("Adding a new tx to the transaction buffer")
	if tx == nil {
		return ErrTxIsNil
	}
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if t.count >= t.maxSize {
		transactionsRejectedCounter.Inc(1)
		return ErrTxBufferFull
	}

	txHash := tx.Hash(t.hashAlgorithm)
	logger.Debug("Transaction hash is %X", txHash)
	txId := string(txHash)

	_, found := t.transactions[txId]
	if found {
		transactionsDuplicateCounter.Inc(1)
		return ErrTxInBuffer
	}
	t.transactions[txId] = tx
	t.count++
	t.transactionsCh <- tx
	transactionsCounter.Inc(1)
	return nil
}

func (t *TxBuffer) Process(ctx context.Context, wg *sync.WaitGroup, process TxHandler) {
	for {
		select {
		case <-ctx.Done():
			wg.Done()
			return
		case tx := <-t.transactionsCh:
			if tx == nil {
				continue
			}
			if process(tx) {
				t.remove(string(tx.Hash(t.hashAlgorithm)))
			}
		}
	}
}

// remove function removes the transaction with given id from the TxBuffer.
func (t *TxBuffer) remove(id string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	_, found := t.transactions[id]
	if found {
		delete(t.transactions, id)
		t.count--
		transactionsCounter.Dec(1)
	}
}

// Count returns the total number of transactions in the TxBuffer.
func (t *TxBuffer) Count() uint32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return t.count
}
