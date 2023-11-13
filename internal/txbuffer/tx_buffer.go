package txbuffer

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

var (
	ErrInvalidMaxSize       = errors.New("invalid maximum size")
	ErrInvalidHashAlgorithm = errors.New("invalid tx hash algorithm")
	ErrTxIsNil              = errors.New("tx is nil")
	ErrTxInBuffer           = errors.New("tx already in tx buffer")
	ErrTxBufferFull         = errors.New("tx buffer is full")
)

type (
	// TxBuffer is an in-memory data structure containing the set of unconfirmed transactions.
	TxBuffer struct {
		mutex          sync.Mutex
		transactions   map[string]time.Time // index of pending transactions, hash->added_ts
		transactionsCh chan *types.TransactionOrder
		hashAlgorithm  crypto.Hash
		log            *slog.Logger

		mCount metric.Int64UpDownCounter
		mDur   metric.Float64Histogram
	}

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
	}
)

/*
New creates a new instance of the TxBuffer.
MaxSize specifies the total number of transactions the TxBuffer may contain.
*/
func New(maxSize uint, hashAlgorithm crypto.Hash, obs Observability, log *slog.Logger) (*TxBuffer, error) {
	if maxSize < 1 {
		return nil, fmt.Errorf("buffer max size must be greater than zero, got %d", maxSize)
	}
	if hashAlgorithm == 0 {
		return nil, ErrInvalidHashAlgorithm
	}

	buf := &TxBuffer{
		hashAlgorithm:  hashAlgorithm,
		transactions:   make(map[string]time.Time),
		transactionsCh: make(chan *types.TransactionOrder, maxSize),
		log:            log,
	}
	if err := buf.initMetrics(obs); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}

	return buf, nil
}

/*
Add adds the given transaction into the transaction buffer.
Returns an error if the transaction is nil, is already present in the TxBuffer,
or TxBuffer is full.
*/
func (buf *TxBuffer) Add(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if tx == nil {
		return nil, ErrTxIsNil
	}

	txHash := tx.Hash(buf.hashAlgorithm)
	buf.log.Debug(fmt.Sprintf("received %s transaction, hash %X", tx.PayloadType(), txHash), logger.UnitID(tx.UnitID()))
	txId := string(txHash)

	buf.mutex.Lock()
	defer buf.mutex.Unlock()

	if _, found := buf.transactions[txId]; found {
		return nil, ErrTxInBuffer
	}

	select {
	case buf.transactionsCh <- tx:
		buf.mCount.Add(ctx, 1)
		buf.transactions[txId] = time.Now()
	default:
		return nil, ErrTxBufferFull
	}

	return txHash, nil
}

func (buf *TxBuffer) Remove(ctx context.Context) (*types.TransactionOrder, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case tx := <-buf.transactionsCh:
		buf.removeFromIndex(ctx, string(tx.Hash(buf.hashAlgorithm)))
		buf.mCount.Add(ctx, -1)
		return tx, nil
	}
}

func (buf *TxBuffer) HashAlgorithm() crypto.Hash {
	return buf.hashAlgorithm
}

/*
removeFromIndex deletes the transaction with given id from the index.
*/
func (buf *TxBuffer) removeFromIndex(ctx context.Context, id string) {
	buf.mutex.Lock()
	defer buf.mutex.Unlock()

	if added, found := buf.transactions[id]; found {
		buf.mDur.Record(ctx, time.Since(added).Seconds())
		delete(buf.transactions, id)
	}
}

func (buf *TxBuffer) initMetrics(obs Observability) (err error) {
	m := obs.Meter("txbuffer")

	if buf.mCount, err = m.Int64UpDownCounter(
		"count",
		metric.WithDescription(`Number of transactions in the buffer.`),
		metric.WithUnit("{transaction}"),
	); err != nil {
		return fmt.Errorf("creating tx counter: %w", err)
	}

	if buf.mDur, err = m.Float64Histogram(
		"queued",
		metric.WithDescription("For how long transaction was in the buffer before being processed."),
		metric.WithUnit("s"),
		//metric.WithExplicitBucketBoundaries(...), // will be in v1.20?
	); err != nil {
		return fmt.Errorf("creating duration histogram: %w", err)
	}

	return nil
}
