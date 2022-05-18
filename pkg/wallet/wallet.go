package wallet

import (
	"errors"
	"sync"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/abclient"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/tyler-smith/go-bip39"
)

const (
	prefetchBlockCount          = 10
	mnemonicEntropyBitSize      = 128
	sleepTimeAtMaxBlockHeightMs = 500
)

var (
	ErrInvalidPassword = errors.New("invalid password")
)

type Wallet struct {
	storage         Storage
	blockProcessor  BlockProcessor
	config          Config
	AlphabillClient abclient.ABClient
	syncFlag        *syncFlagWrapper
}

// CreateNewWallet creates a new wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func CreateNewWallet(storage Storage, blockProcessor BlockProcessor, config Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}
	mnemonic, err := generateMnemonic()
	if err != nil {
		return nil, err
	}
	return createWallet(storage, blockProcessor, mnemonic, config)
}

// CreateWalletFromSeed creates a new wallet from given seed mnemonic. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func CreateWalletFromSeed(storage Storage, blockProcessor BlockProcessor, mnemonic string, config Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}
	return createWallet(storage, blockProcessor, mnemonic, config)
}

// LoadExistingWallet loads an existing wallet. To synchronize wallet with a node call Sync.
// Shutdown needs to be called to release resources used by wallet.
func LoadExistingWallet(storage Storage, blockProcessor BlockProcessor, config Config) (*Wallet, error) {
	err := log.InitDefaultLogger()
	if err != nil {
		return nil, err
	}
	ok, err := storage.VerifyPassword()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrInvalidPassword
	}
	return newWallet(storage, blockProcessor, config), nil
}

// GetPublicKey returns public key of the wallet (compressed secp256k1 key 33 bytes)
func (w *Wallet) GetPublicKey() ([]byte, error) {
	key, err := w.storage.GetAccountKey()
	if err != nil {
		return nil, err
	}
	return key.PubKey, nil
}

// GetMnemonic returns mnemonic seed of the wallet
func (w *Wallet) GetMnemonic() (string, error) {
	return w.storage.GetMnemonic()
}

// Sync synchronises wallet with given alphabill node.
// The function blocks forever or until alphabill connection is terminated.
// Returns immediately if already synchronizing.
func (w *Wallet) Sync() {
	w.syncLedger(true)
}

// SyncToMaxBlockHeight synchronises wallet with given alphabill node.
// The function blocks until maximum block height, calculated at the start of the process, is reached.
// Returns immediately if already synchronizing.
func (w *Wallet) SyncToMaxBlockHeight() {
	w.syncLedger(false)
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
	// TODO deleegate shutdown to blockProcessor and walletStorage?
}

//// DeleteDb deletes the wallet database.
//func (w *Wallet) DeleteDb() {
//	// TODO delegate to storage or move function to storage?
//}

func newWallet(storage Storage, blockProcessor BlockProcessor, config Config) *Wallet {
	return &Wallet{
		storage:        storage,
		blockProcessor: blockProcessor,
		config:         config,
		AlphabillClient: abclient.New(abclient.AlphabillClientConfig{
			Uri:              config.AlphabillClientConfig.Uri,
			RequestTimeoutMs: config.AlphabillClientConfig.RequestTimeoutMs,
		}),
		syncFlag: newSyncFlagWrapper(),
	}
}

// syncLedger downloads and processes blocks, blocks until error in rpc connection
func (w *Wallet) syncLedger(syncForever bool) {
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
			err = w.fetchBlocksForever(ch)
		} else {
			err = w.fetchBlocksUntilMaxBlock(ch)
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

func (w *Wallet) fetchBlocksForever(ch chan<- *block.Block) error {
	blockNo, err := w.storage.GetBlockHeight()
	if err != nil {
		return err
	}
	for {
		select {
		case <-w.syncFlag.cancelSyncCh:
			return nil
		default:
			maxBlockNo, err := w.AlphabillClient.GetMaxBlockNo()
			if err != nil {
				return err
			}
			if blockNo == maxBlockNo {
				time.Sleep(sleepTimeAtMaxBlockHeightMs * time.Millisecond)
				continue
			}
			blockNo = blockNo + 1
			b, err := w.AlphabillClient.GetBlock(blockNo)
			if err != nil {
				return err
			}
			ch <- b
		}
	}
}

func (w *Wallet) fetchBlocksUntilMaxBlock(ch chan<- *block.Block) error {
	blockNo, err := w.storage.GetBlockHeight()
	if err != nil {
		return err
	}
	maxBlockNo, err := w.AlphabillClient.GetMaxBlockNo()
	if err != nil {
		return err
	}
	for blockNo < maxBlockNo {
		select {
		case <-w.syncFlag.cancelSyncCh:
			return nil
		default:
			blockNo = blockNo + 1
			b, err := w.AlphabillClient.GetBlock(blockNo)
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
		err = w.blockProcessor.PostProcessBlock(b)
		if err != nil {
			return err
		}
	}
	return nil
}

func createWallet(storage Storage, blockProcessor BlockProcessor, mnemonic string, config Config) (*Wallet, error) {
	k, err := generateKeys(mnemonic)
	if err != nil {
		return nil, err
	}
	err = storage.SetEncrypted(config.WalletPass != "")
	if err != nil {
		return nil, err
	}
	err = storage.SetMnemonic(mnemonic)
	if err != nil {
		return nil, err
	}
	err = storage.SetMasterKey(k.masterKey.String())
	if err != nil {
		return nil, err
	}
	err = storage.SetAccountKey(k.accountKey)
	if err != nil {
		return nil, err
	}
	return newWallet(storage, blockProcessor, config), nil
}

func generateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(mnemonicEntropyBitSize)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}
