package wallet

import (
	"sync"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/abclient"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/tyler-smith/go-bip39"
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
// Returns immediately if already synchronizing.
func (w *Wallet) Sync(blockNumber uint64) {
	w.syncLedger(blockNumber, true)
}

// Sync synchronises wallet from the last known block number with the given alphabill node.
// The function blocks until maximum block height, calculated at the start of the process, is reached.
// Returns immediately if already synchronizing.
func (w *Wallet) SyncToMaxBlockNumber(blockNumber uint64) {
	w.syncLedger(blockNumber, false)
}

// GetMaxBlockNumber queries the node for latest block number
func (w *Wallet) GetMaxBlockNumber() (uint64, error) {
	return w.AlphabillClient.GetMaxBlockNumber()
}

// SendTransaction broadcasts transaction to configured node.
func (w *Wallet) SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
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
		w.AlphabillClient.Shutdown()
	}
}

// syncLedger downloads and processes blocks, blocks until error in rpc connection
func (w *Wallet) syncLedger(blockNumber uint64, syncForever bool) {
	if w.syncFlag.isSynchronizing() {
		log.Warning("wallet is already synchronizing")
		return
	}
	log.Info("starting ledger synchronization process")
	w.syncFlag.setSynchronizing(true)
	defer w.syncFlag.setSynchronizing(false)

	var wg sync.WaitGroup
	wg.Add(2)
	ch := make(chan *block.Block, prefetchBlockCount)
	go func() {
		var err error
		if syncForever {
			err = w.fetchBlocksForever(blockNumber, ch)
		} else {
			err = w.fetchBlocksUntilMaxBlock(blockNumber, ch)
		}
		if err != nil {
			log.Error("error fetching block: ", err)
		}
		log.Info("closing block receiver channel")
		close(ch)
		wg.Done()
	}()
	go func() {
		err := w.processBlocks(ch)
		if err != nil {
			log.Error("error processing block: ", err)
			// signal block receiver goroutine to stop fetching blocks
			w.syncFlag.cancelSyncCh <- true
		} else {
			log.Info("block processor channel closed")
		}
		wg.Done()
	}()
	wg.Wait()
	log.Info("ledger sync finished")
}

func (w *Wallet) fetchBlocksForever(blockNumber uint64, ch chan<- *block.Block) error {
	for {
		select {
		case <-w.syncFlag.cancelSyncCh:
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

func (w *Wallet) fetchBlocksUntilMaxBlock(blockNumber uint64, ch chan<- *block.Block) error {
	maxBlockNo, err := w.GetMaxBlockNumber()
	if err != nil {
		return err
	}
	for blockNumber < maxBlockNo {
		select {
		case <-w.syncFlag.cancelSyncCh:
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
		err := w.processBlock(b)
		if err != nil {
			errRollback := w.blockProcessor.Rollback()
			if errRollback != nil {
				log.Error("error reverting block %v in block processor", errRollback)
			}
			return err
		}
		err = w.blockProcessor.Commit()
		if err != nil {
			return err
		}
	}
	return nil
}

func (w *Wallet) processBlock(b *block.Block) error {
	err := w.blockProcessor.BeginBlock(b.BlockNumber)
	if err != nil {
		return err
	}
	for _, tx := range b.Transactions {
		err = w.blockProcessor.ProcessTx(tx)
		if err != nil {
			return err
		}
	}
	return w.blockProcessor.EndBlock()
}

func createWallet(blockProcessor BlockProcessor, mnemonic string, config Config) (*Wallet, *Keys, error) {
	k, err := generateKeys(mnemonic)
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
