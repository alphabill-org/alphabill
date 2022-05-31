package wallet

import (
	"context"
	"time"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/sync/errgroup"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/abclient"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
)

const (
	prefetchBlockCount          = 10
	mnemonicEntropyBitSize      = 128
	sleepTimeAtMaxBlockHeightMs = 500
)

type Wallet struct {
	blockProcessor  BlockProcessor
	config          Config
	AlphabillClient abclient.ABClient
	syncFlag        *syncFlagWrapper
}

// NewEmptyWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func NewEmptyWallet(blockProcessor BlockProcessor, config Config, mnemonic string) (*Wallet, *Keys, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, nil, err
	}
	if mnemonic == "" {
		mnemonic, err = generateMnemonic()
		if err != nil {
			return nil, nil, err
		}
	}
	return createWallet(blockProcessor, mnemonic, config)
}

// NewExistingWallet loads an existing wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func NewExistingWallet(blockProcessor BlockProcessor, config Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}
	return newWallet(blockProcessor, config), nil
}

func newWallet(blockProcessor BlockProcessor, config Config) *Wallet {
	return &Wallet{
		blockProcessor: blockProcessor,
		config:         config,
		AlphabillClient: abclient.New(abclient.AlphabillClientConfig{
			Uri:              config.AlphabillClientConfig.Uri,
			RequestTimeoutMs: config.AlphabillClientConfig.RequestTimeoutMs,
		}),
		syncFlag: newSyncFlagWrapper(),
	}
}

// Sync synchronises wallet from the last known block number with the given alphabill node.
// The function blocks forever or until alphabill connection is terminated.
// Returns error if wallet is already synchronizing or any error occured during syncrohronization, otherwise returns nil.
func (w *Wallet) Sync(blockNumber uint64) error {
	return w.syncLedger(blockNumber, true)
}

// Sync synchronises wallet from the last known block number with the given alphabill node.
// The function blocks until maximum block height, calculated at the start of the process, is reached.
// Returns error if wallet is already synchronizing or any error occured during syncrohronization, otherwise returns nil.
func (w *Wallet) SyncToMaxBlockNumber(blockNumber uint64) error {
	return w.syncLedger(blockNumber, false)
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

	// cancel synchronization process
	w.syncFlag.ctxCancelFunc()

	if w.AlphabillClient != nil {
		err := w.AlphabillClient.Shutdown()
		if err != nil {
			log.Error("error shutting down wallet: ", err)
		}
	}
}

// syncLedger downloads and processes blocks, blocks until error in rpc connection
func (w *Wallet) syncLedger(blockNumber uint64, syncForever bool) error {
	if w.syncFlag.isSynchronizing() {
		return errors.New("wallet is already synchronizing")
	}
	log.Info("starting ledger synchronization process")
	w.syncFlag.setSynchronizing(true)
	defer w.syncFlag.setSynchronizing(false)

	ch := make(chan *block.Block, prefetchBlockCount)
	errGroup, ctx := errgroup.WithContext(w.syncFlag.ctx)
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
		case <-ctx.Done(): // context is cancelled from shutdown or error in block receiver
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
		case <-ctx.Done(): // context is cancelled from shutdown or error in block receiver
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
		err := w.blockProcessor.ProcessBlock(b)
		if err != nil {
			return err
		}
	}
	return nil
}

func createWallet(blockProcessor BlockProcessor, mnemonic string, config Config) (*Wallet, *Keys, error) {
	k, err := NewKeys(mnemonic)
	if err != nil {
		return nil, nil, err
	}
	return newWallet(blockProcessor, config), k, nil
}

func generateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(mnemonicEntropyBitSize)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}
