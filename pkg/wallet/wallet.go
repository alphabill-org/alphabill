package wallet

import (
	"context"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/client"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"golang.org/x/sync/errgroup"
)

const (
	prefetchBlockCount          = 10
	sleepTimeAtMaxBlockHeightMs = 500
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
func (w *Wallet) Sync(ctx context.Context, blockNumber uint64) error {
	return w.syncLedger(ctx, blockNumber, true)
}

// SyncToMaxBlockNumber synchronises wallet from the last known block number with the given alphabill node.
// The function blocks until maximum block height, calculated at the start of the process, is reached.
// Returns error if wallet is already synchronizing or any error occured during syncrohronization, otherwise returns nil.
func (w *Wallet) SyncToMaxBlockNumber(ctx context.Context, blockNumber uint64) error {
	return w.syncLedger(ctx, blockNumber, false)
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
func (w *Wallet) syncLedger(ctx context.Context, blockNumber uint64, syncForever bool) error {
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
			err = w.fetchBlocksForever(ctx, blockNumber, ch)
		} else {
			err = w.fetchBlocksUntilMaxBlock(ctx, blockNumber, ch)
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

func (w *Wallet) fetchBlocksForever(ctx context.Context, blockNumber uint64, ch chan<- *block.Block) error {
	for {
		select {
		case <-w.syncFlag.cancelSyncCh: // canceled from shutdown
			return nil
		case <-ctx.Done(): // canceled by user or error in block receiver
			return nil
		default:
			maxBlockNo, err := w.GetMaxBlockNumber()
			if err != nil {
				return err
			}
			if blockNumber == maxBlockNo {
				time.Sleep(sleepTimeAtMaxBlockHeightMs * time.Millisecond)
				continue
			}
			blockNumber = blockNumber + 1
			b, err := w.AlphabillClient.GetBlock(blockNumber)
			if err != nil {
				return err
			}
			ch <- b
		}
	}
}

func (w *Wallet) fetchBlocksUntilMaxBlock(ctx context.Context, blockNumber uint64, ch chan<- *block.Block) error {
	maxBlockNo, err := w.GetMaxBlockNumber()
	if err != nil {
		return err
	}
	for blockNumber < maxBlockNo {
		select {
		case <-w.syncFlag.cancelSyncCh: // canceled from shutdown
			return nil
		case <-ctx.Done(): // canceled by user or error in block receiver
			return nil
		default:
			blockNumber = blockNumber + 1
			b, err := w.AlphabillClient.GetBlock(blockNumber)
			if err != nil {
				return err
			}
			ch <- b
		}
	}
	return nil
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
