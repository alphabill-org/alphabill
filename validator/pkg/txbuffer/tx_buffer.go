package txbuffer

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/common/logger"
	"github.com/alphabill-org/alphabill/validator/pkg/metrics"
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
		transactions   map[string]struct{} // index of pending transactions
		transactionsCh chan *types.TransactionOrder
		hashAlgorithm  crypto.Hash
		log            *slog.Logger
	}

	// TxHandler processes an transaction.
	TxHandler func(ctx context.Context, tx *types.TransactionOrder)
)

/*
New creates a new instance of the TxBuffer.
MaxSize specifies the total number of transactions the TxBuffer may contain.
*/
func New(maxSize uint32, hashAlgorithm crypto.Hash, log *slog.Logger) (*TxBuffer, error) {
	if maxSize < 1 {
		return nil, ErrInvalidMaxSize
	}
	return &TxBuffer{
		hashAlgorithm:  hashAlgorithm,
		transactions:   make(map[string]struct{}),
		transactionsCh: make(chan *types.TransactionOrder, maxSize),
		log:            log,
	}, nil
}

/*
Add adds the given transaction into the transaction buffer.
Returns an error if the transaction isn't valid, is already present in the TxBuffer,
or TxBuffer is full.
*/
func (t *TxBuffer) Add(tx *types.TransactionOrder) ([]byte, error) {
	if tx == nil {
		return nil, ErrTxIsNil
	}

	txHash := tx.Hash(t.hashAlgorithm)
	t.log.Debug(fmt.Sprintf("received %s transaction, hash %X", tx.PayloadType(), txHash), logger.UnitID(tx.UnitID()))
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
		t.transactions[txId] = struct{}{}
	default:
		transactionsRejectedCounter.Inc(1)
		return nil, ErrTxBufferFull
	}

	return txHash, nil
}

/*
Process calls the "process" callback for each transaction in the buffer until
ctx is cancelled.
After callback returns the tx is always removed from internal buffer (ie the
callback can't add the tx back to buffer, it would be rejected as duplicate).
*/
func (t *TxBuffer) Process(ctx context.Context, process TxHandler) {
	for {
		select {
		case <-ctx.Done():
			return
		case tx := <-t.transactionsCh:
			process(ctx, tx)
			t.removeFromIndex(string(tx.Hash(t.hashAlgorithm)))
		}
	}
}

/*
removeFromIndex deletes the transaction with given id from the index.
*/
func (t *TxBuffer) removeFromIndex(id string) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if _, found := t.transactions[id]; found {
		delete(t.transactions, id)
		transactionsCounter.Dec(1)
	}
}
