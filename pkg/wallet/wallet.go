package wallet

import (
	"context"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"golang.org/x/sync/errgroup"
)

const (
	prefetchBlockCount          = 100
	sleepTimeAtMaxBlockHeightMs = 500
	blockDownloadMaxBatchSize   = 100
)

var (
	ErrWalletAlreadySynchronizing = errors.New("wallet is already synchronizing")
)

// Wallet To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
type Wallet struct {
	blockProcessor  BlockProcessor
	config          Config
	AlphabillClient client.ABClient
	syncFlag        *syncFlagWrapper
}

type Builder struct {
	bp      BlockProcessor
	abcConf client.AlphabillClientConfig
	// overrides abcConf
	abc client.ABClient
}

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
	w := &Wallet{syncFlag: newSyncFlagWrapper()}
	w.blockProcessor = b.bp
	if b.abc != nil {
		w.AlphabillClient = b.abc
	} else {
		w.AlphabillClient = client.New(b.abcConf)
	}
	return w
}

// Sync synchronises wallet from the last known block number with the given alphabill node.
// The function blocks forever or until alphabill connection is terminated.
// Returns error if wallet is already synchronizing or any error occured during syncrohronization, otherwise returns nil.
func (w *Wallet) Sync(ctx context.Context, lastBlockNumber uint64) error {
	return w.syncLedger(ctx, lastBlockNumber, true)
}

// SyncToMaxBlockNumber synchronises wallet from the last known block number with the given alphabill node.
// The function blocks until maximum block height, calculated at the start of the process, is reached.
// Returns error if wallet is already synchronizing or any error occured during syncrohronization, otherwise returns nil.
func (w *Wallet) SyncToMaxBlockNumber(ctx context.Context, lastBlockNumber uint64) error {
	return w.syncLedger(ctx, lastBlockNumber, false)
}

// GetMaxBlockNumber queries the node for latest block number
func (w *Wallet) GetMaxBlockNumber() (uint64, error) {
	return w.AlphabillClient.GetMaxBlockNumber()
}

// SendTransaction broadcasts transaction to configured node.
func (w *Wallet) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	return w.AlphabillClient.SendTransaction(tx)
}

// Shutdown terminates connection to alphabill node, closes wallet db, cancels dust collector job and any background goroutines.
func (w *Wallet) Shutdown() {
	log.Info("shutting down wallet")

	// send cancel signal only if channel is not full
	// this check is needed in case Shutdown is called multiple times
	// alternatively we can prohibit reusing wallet that has been shut down
	select {
	case w.syncFlag.cancelSyncCh <- true:
	default:
	}

	if w.AlphabillClient != nil {
		err := w.AlphabillClient.Shutdown()
		if err != nil {
			log.Error("error shutting down wallet: ", err)
		}
	}
}

// syncLedger downloads and processes blocks, blocks until error in rpc connection
func (w *Wallet) syncLedger(ctx context.Context, lastBlockNumber uint64, syncForever bool) error {
	if w.syncFlag.isSynchronizing() {
		return ErrWalletAlreadySynchronizing
	}
	log.Info("starting ledger synchronization process")
	w.syncFlag.setSynchronizing(true)
	defer w.syncFlag.setSynchronizing(false)

	ch := make(chan *block.Block, prefetchBlockCount)

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		var err error
		if syncForever {
			err = w.fetchBlocksForever(ctx, lastBlockNumber, ch)
		} else {
			err = w.fetchBlocksUntilMaxBlock(ctx, lastBlockNumber, ch)
		}
		log.Info("closing block receiver channel")
		close(ch)

		log.Info("block receiver goroutine finished")
		return err
	})
	errGroup.Go(func() error {
		err := w.processBlocks(ch)
		log.Info("block processor goroutine finished")
		return err
	})
	err := errGroup.Wait()
	log.Info("ledger sync finished")
	return err
}

func (w *Wallet) fetchBlocksForever(ctx context.Context, lastBlockNumber uint64, ch chan<- *block.Block) error {
	log.Info("syncing from current block number ", lastBlockNumber)
	var err error
	var maxBlockNumber uint64
	for {
		select {
		case <-w.syncFlag.cancelSyncCh: // canceled from shutdown
			return nil
		case <-ctx.Done(): // canceled by user or error in block receiver
			return nil
		default:
			if maxBlockNumber != 0 && lastBlockNumber == maxBlockNumber {
				time.Sleep(sleepTimeAtMaxBlockHeightMs * time.Millisecond)
			}
			lastBlockNumber, maxBlockNumber, err = w.fetchBlocks(lastBlockNumber, blockDownloadMaxBatchSize, ch)
			if err != nil {
				return err
			}
		}
	}
}

func (w *Wallet) fetchBlocksUntilMaxBlock(ctx context.Context, lastBlockNumber uint64, ch chan<- *block.Block) error {
	maxBlockNumber, err := w.GetMaxBlockNumber()
	if err != nil {
		return err
	}
	log.Info("syncing from current block number ", lastBlockNumber, " to ", maxBlockNumber)
	for lastBlockNumber < maxBlockNumber {
		select {
		case <-w.syncFlag.cancelSyncCh: // canceled from shutdown
			return nil
		case <-ctx.Done(): // canceled by user or error in block receiver
			return nil
		default:
			batchSize := util.Min(blockDownloadMaxBatchSize, maxBlockNumber-lastBlockNumber)
			lastBlockNumber, _, err = w.fetchBlocks(lastBlockNumber, batchSize, ch)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Wallet) fetchBlocks(lastBlockNumber uint64, batchSize uint64, ch chan<- *block.Block) (uint64, uint64, error) {
	log.Debug("fetching blocks blocknumber=", lastBlockNumber+1, " blockcount=", batchSize)
	res, err := w.AlphabillClient.GetBlocks(lastBlockNumber+1, batchSize)
	if err != nil {
		return 0, 0, err
	}
	for _, b := range res.Blocks {
		lastBlockNumber = b.BlockNumber
		ch <- b
	}
	return lastBlockNumber, res.MaxBlockNumber, nil
}

func (w *Wallet) processBlocks(ch <-chan *block.Block) error {
	for b := range ch {
		if w.blockProcessor != nil {
			err := w.blockProcessor.ProcessBlock(b)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
