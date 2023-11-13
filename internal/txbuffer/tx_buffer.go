package txbuffer

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

var (
	ErrTxBufferFull         = errors.New("tx buffer is full")
	ErrInvalidMaxSize       = errors.New("invalid maximum size")
	ErrInvalidHashAlgorithm = errors.New("invalid maximum size")
	ErrTxIsNil              = errors.New("tx is nil")
	ErrTxInBuffer           = errors.New("tx already in tx buffer")

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
)

/*
New creates a new instance of the TxBuffer.
MaxSize specifies the total number of transactions the TxBuffer may contain.
*/
func New(maxSize uint, hashAlgorithm crypto.Hash, log *slog.Logger) (*TxBuffer, error) {
	if maxSize < 1 {
		return nil, ErrInvalidMaxSize
	}
	if hashAlgorithm == 0 {
		return nil, ErrInvalidHashAlgorithm
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

func (t *TxBuffer) Remove(ctx context.Context) (*types.TransactionOrder, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case tx := <-t.transactionsCh:
		t.removeFromIndex(string(tx.Hash(t.hashAlgorithm)))
		return tx, nil
	}
}

func (t *TxBuffer) HashAlgorithm() crypto.Hash {
	return t.hashAlgorithm
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
