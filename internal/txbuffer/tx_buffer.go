package txbuffer

import (
	"context"
	gocrypto "crypto"
	"errors"
	"sync"

	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/types"
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
		mutex          sync.Mutex
		transactions   map[string]*types.TransactionOrder // map containing valid pending transactions.
		transactionsCh chan *types.TransactionOrder
		hashAlgorithm  gocrypto.Hash
	}

	// TxHandler handles the transaction. Return value should indicate whether the tx was processed
	// successfully (and thus removed from buffer) but currently this value is ignored - after callback
	// returns the tx is always removed from internal buffer.
	TxHandler func(ctx context.Context, tx *types.TransactionOrder) bool
)

// New creates a new instance of the TxBuffer. MaxSize specifies the total number of transactions the TxBuffer may
// contain.
func New(maxSize uint32, hashAlgorithm gocrypto.Hash) (*TxBuffer, error) {
	if maxSize < 1 {
		return nil, ErrInvalidMaxSize
	}
	return &TxBuffer{
		hashAlgorithm:  hashAlgorithm,
		transactions:   make(map[string]*types.TransactionOrder),
		transactionsCh: make(chan *types.TransactionOrder, maxSize),
	}, nil
}

func (t *TxBuffer) Close() {
	close(t.transactionsCh)
}

func (t *TxBuffer) Capacity() uint32 {
	return uint32(cap(t.transactionsCh))
}

// Add adds the given transaction to the transaction buffer. Returns an error if the transaction isn't valid, is
// already present in the TxBuffer, or TxBuffer is full.
func (t *TxBuffer) Add(tx *types.TransactionOrder) ([]byte, error) {
	if tx == nil {
		return nil, ErrTxIsNil
	}

	txHash := tx.Hash(t.hashAlgorithm)
	logger.Debug("Transaction hash is %X", txHash)
	txId := string(txHash)

	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, found := t.transactions[txId]; found {
		transactionsDuplicateCounter.Inc(1)
		return nil, ErrTxInBuffer
	}

	select {
	case t.transactionsCh <- tx:
		transactionsCounter.Inc(1)
		t.transactions[txId] = tx
	default:
		transactionsRejectedCounter.Inc(1)
		return nil, ErrTxBufferFull
	}

	return txHash, nil
}

/*
Process calls the "process" callback for a transaction. Return value of the callback should indicate
whether the tx was processed successfully or not but currently this value is ignored - after callback
returns the tx is always removed from internal buffer.
*/
func (t *TxBuffer) Process(ctx context.Context, process TxHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		case tx, ok := <-t.transactionsCh:
			if !ok {
				return
			}
			process(ctx, tx)
			t.removeFromIndex(string(tx.Hash(t.hashAlgorithm)))
		}
	}
}

// removeFromIndex deletes the transaction with given id from the index (note that
// tx still might be in the channel!)
func (t *TxBuffer) removeFromIndex(id string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, found := t.transactions[id]; found {
		delete(t.transactions, id)
		transactionsCounter.Dec(1)
	}
}

// Count returns the total number of transactions in the TxBuffer.
func (t *TxBuffer) Count() uint32 {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return uint32(len(t.transactionsCh))
}
