package wallet

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"golang.org/x/sync/errgroup"
)

const (
	prefetchBlockCount          = 100
	sleepTimeAtMaxBlockHeightMs = 500
	blockDownloadMaxBatchSize   = 100
	maxTxFailedTries            = 3
	txBufferFullErrMsg          = "tx buffer is full"
)

var (
	ErrFailedToBroadcastTx = errors.New("failed to broadcast transaction")
	ErrTxRetryCanceled     = errors.New("user canceled tx retry")
)

type (
	// Wallet To synchronize wallet with a node call Sync.
	// Shutdown needs to be called to release resources used by wallet.
	Wallet struct {
		BlockProcessor  BlockProcessor
		AlphabillClient client.ABClient
	}
	Builder struct {
		bp      BlockProcessor
		abcConf client.AlphabillClientConfig
		// overrides abcConf
		abc client.ABClient
	}
	SendOpts struct {
		// RetryOnFullTxBuffer retries to send transaction when tx buffer is full
		RetryOnFullTxBuffer bool
	}

	fetchBlocksResult struct {
		lastFetchedBlockNumber  uint64 // latest processed block by a wallet/client
		maxAvailableBlockNumber uint64 // latest non-empty block in a partition shard
		maxAvailableRoundNumber uint64 // latest round number in a partition shard, greater or equal to maxAvailableBlockNumber
	}
)

func New() *Builder {
	return &Builder{}
}

func (b *Builder) SetBlockProcessor(bp BlockProcessor) *Builder {
	b.bp = bp
	return b
}

func (b *Builder) SetABClientConf(abcConf client.AlphabillClientConfig) *Builder {
	b.abcConf = abcConf
	return b
}

func (b *Builder) SetABClient(abc client.ABClient) *Builder {
	b.abc = abc
	return b
}

func (b *Builder) Build() *Wallet {
	return &Wallet{
		AlphabillClient: b.getOrCreateABClient(),
		BlockProcessor:  b.bp,
	}
}

func (b *Builder) getOrCreateABClient() client.ABClient {
	if b.abc != nil {
		return b.abc
	}
	return client.New(b.abcConf)
}

// Sync synchronises wallet from the last known block number with the given alphabill node.
// The function blocks forever or until alphabill connection is terminated.
// Returns error if wallet is already synchronizing or any error occured during syncrohronization, otherwise returns nil.
func (w *Wallet) Sync(ctx context.Context, lastBlockNumber uint64) error {
	return w.syncLedger(ctx, lastBlockNumber, true)
}

// GetRoundNumber queries the node for latest round number
func (w *Wallet) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.AlphabillClient.GetRoundNumber(ctx)
}

// SendTransaction broadcasts transaction to configured node.
// Returns nil if transaction was successfully accepted by node, otherwise returns error.
func (w *Wallet) SendTransaction(ctx context.Context, tx *types.TransactionOrder, opts *SendOpts) error {
	if opts == nil || !opts.RetryOnFullTxBuffer {
		return w.sendTx(ctx, tx, 1)
	}
	return w.sendTx(ctx, tx, maxTxFailedTries)
}

// Shutdown terminates connection to alphabill node and cancels any background goroutines.
func (w *Wallet) Shutdown() {
	log.Debug("shutting down wallet")

	if w.AlphabillClient != nil {
		err := w.AlphabillClient.Shutdown()
		if err != nil {
			log.Error("error shutting down wallet: ", err)
		}
	}
}

// syncLedger downloads and processes blocks, blocks until error in rpc connection
func (w *Wallet) syncLedger(ctx context.Context, lastBlockNumber uint64, syncForever bool) error {
	log.Info("starting ledger synchronization process")

	ch := make(chan *types.Block, prefetchBlockCount)

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		var err error
		if syncForever {
			err = w.fetchBlocksForever(ctx, lastBlockNumber, ch)
		} else {
			err = w.fetchBlocksUntilMaxBlock(ctx, lastBlockNumber, ch)
		}
		log.Debug("closing block receiver channel")
		close(ch)

		log.Debug("block receiver goroutine finished")
		return err
	})
	errGroup.Go(func() error {
		err := w.processBlocks(ch)
		log.Debug("block processor goroutine finished")
		return err
	})
	err := errGroup.Wait()
	log.Info("ledger sync finished")
	return err
}

func (w *Wallet) fetchBlocksForever(ctx context.Context, lastBlockNumber uint64, ch chan<- *types.Block) error {
	log.Info("syncing until cancelled from current block number ", lastBlockNumber)
	var maxBlockNumber uint64
	for {
		select {
		case <-ctx.Done(): // canceled by user or error in block receiver
			return nil
		default:
			if maxBlockNumber != 0 && lastBlockNumber == maxBlockNumber {
				// wait for some time before retrying to fetch new block
				select {
				case <-ctx.Done():
					return nil
				case <-time.After(sleepTimeAtMaxBlockHeightMs * time.Millisecond):
				}
			}
			res, err := w.fetchBlocks(ctx, lastBlockNumber, blockDownloadMaxBatchSize, ch) // TODO: merge
			if err != nil {
				return err
			}
			lastBlockNumber = res.lastFetchedBlockNumber
			maxBlockNumber = res.maxAvailableBlockNumber
		}
	}
}

func (w *Wallet) fetchBlocksUntilMaxBlock(ctx context.Context, lastBlockNumber uint64, ch chan<- *types.Block) error {
	maxBlockNumber, err := w.GetRoundNumber(ctx)
	if err != nil {
		return err
	}
	log.Info("syncing from current block number ", lastBlockNumber, " to ", maxBlockNumber)
	for lastBlockNumber < maxBlockNumber {
		select {
		case <-ctx.Done(): // canceled by user or error in block receiver
			return nil
		default:
			batchSize := util.Min(blockDownloadMaxBatchSize, maxBlockNumber-lastBlockNumber)
			res, err := w.fetchBlocks(ctx, lastBlockNumber, batchSize, ch)
			if err != nil {
				return err
			}
			lastBlockNumber = res.lastFetchedBlockNumber
		}
	}
	return nil
}

func (w *Wallet) fetchBlocks(ctx context.Context, lastBlockNumber uint64, batchSize uint64, ch chan<- *types.Block) (*fetchBlocksResult, error) {
	fromBlockNumber := lastBlockNumber + 1
	res, err := w.AlphabillClient.GetBlocks(ctx, fromBlockNumber, batchSize)
	if err != nil {
		return nil, err
	}
	result := &fetchBlocksResult{
		lastFetchedBlockNumber:  lastBlockNumber,
		maxAvailableBlockNumber: res.MaxBlockNumber,
		maxAvailableRoundNumber: res.MaxRoundNumber,
	}
	for _, b := range res.Blocks {
		block := &types.Block{}
		if err := cbor.Unmarshal(b, block); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block: %w", err)
		}
		result.lastFetchedBlockNumber = block.UnicityCertificate.InputRecord.RoundNumber
		ch <- block
	}
	// this makes sure empty blocks are taken into account (the whole batch might be empty in fact)
	if res.BatchMaxBlockNumber > result.lastFetchedBlockNumber {
		result.lastFetchedBlockNumber = res.BatchMaxBlockNumber
	}
	return result, nil
}

func (w *Wallet) processBlocks(ch <-chan *types.Block) error {
	for b := range ch {
		if w.BlockProcessor != nil {
			err := w.BlockProcessor.ProcessBlock(b)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Wallet) sendTx(ctx context.Context, tx *types.TransactionOrder, maxRetries int) error {
	for failedTries := 0; failedTries < maxRetries; failedTries++ {
		err := w.AlphabillClient.SendTransaction(ctx, tx)
		if err == nil {
			return nil
		}
		// error message can also contain stacktrace when node returns aberror, so we check prefix instead of exact match
		if strings.HasPrefix(err.Error(), txBufferFullErrMsg) {
			log.Debug("tx buffer full, waiting 1s to retry...")
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return ErrTxRetryCanceled
			}
		}
		return fmt.Errorf("failed to send transaction: %w", err)
	}
	return ErrFailedToBroadcastTx
}
