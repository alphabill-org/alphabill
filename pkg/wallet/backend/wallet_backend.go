package backend

import (
	"context"
	"sync"
	"time"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/proof"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
)

var alphabillMoneySystemId = []byte{0, 0, 0, 0}

type (
	WalletBackend struct {
		store         BillStore
		genericWallet *wallet.Wallet

		mu   sync.Mutex // mu mutex guarding sync flag
		sync bool       // sync true if wallet is synchronizing or trying to synchronize, guarded by mu
	}

	bill struct {
		Id    *uint256.Int
		Value uint64
	}

	blockProof struct {
		BillId      []byte            `json:"billId"`
		BlockNumber uint64            `json:"blockNumber"`
		BlockProof  *proof.BlockProof `json:"blockProof"`
	}

	pubkey struct {
		pubkey     []byte
		pubkeyHash *wallet.KeyHashes
	}

	BillStore interface {
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error
		GetBills(pubKey []byte) ([]*bill, error)
		AddBill(pubKey []byte, bill *bill) error
		RemoveBill(pubKey []byte, id *uint256.Int) error
		ContainsBill(pubKey []byte, id *uint256.Int) (bool, error)
		GetBlockProof(billId []byte) (*blockProof, error)
		SetBlockProof(proof *blockProof) error
	}
)

// New creates a new wallet backend service which can be started by calling the Start or StartProcess method.
// Shutdown method should be called to close resources used by the service.
func New(pubkeys [][]byte, abclient client.ABClient, store BillStore) *WalletBackend {
	var trackedPubKeys []*pubkey
	for _, pk := range pubkeys {
		trackedPubKeys = append(trackedPubKeys, &pubkey{
			pubkey: pk,
			pubkeyHash: &wallet.KeyHashes{
				Sha256: hash.Sum256(pk),
				Sha512: hash.Sum512(pk),
			},
		})
	}
	bp := newBlockProcessor(store, trackedPubKeys)
	genericWallet := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()
	return &WalletBackend{store: store, genericWallet: genericWallet}
}

// Start starts downloading blocks and indexing bills by their owner's public key.
// Blocks forever or until alphabill connection is terminated.
func (w *WalletBackend) Start(ctx context.Context) error {
	blockNumber, err := w.store.GetBlockNumber()
	if err != nil {
		return err
	}
	return w.genericWallet.Sync(ctx, blockNumber)
}

// StartProcess same as Start but in case of error restarts the process.
// Returns immediately and retries until canceled by Shutdown method.
func (w *WalletBackend) StartProcess(ctx context.Context) *sync.WaitGroup {
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		wlog.Info("starting wallet synchronization goroutine")
		w.setSync(true)
		retryCount := 0
		for w.isSync() {
			if retryCount > 0 {
				wlog.Info("sleeping 10s before retrying alphabill connection")
				time.Sleep(10 * time.Second)
			}
			err := w.Start(ctx)
			if err != nil {
				wlog.Error("error synchronizing wallet: ", err)
			}
			retryCount++
		}
		wlog.Info("wallet synchronization goroutine ended")
		wg.Done()
	}()
	return &wg
}

// GetBills returns all bills for given public key.
func (w *WalletBackend) GetBills(pubkey []byte) ([]*bill, error) {
	return w.store.GetBills(pubkey)
}

// GetBlockProof returns most recent proof for given unit id.
func (w *WalletBackend) GetBlockProof(unitId []byte) (*blockProof, error) {
	return w.store.GetBlockProof(unitId)
}

// Shutdown terminates wallet backend service.
func (w *WalletBackend) Shutdown() {
	w.setSync(false)
	w.genericWallet.Shutdown()
}

func (w *WalletBackend) isSync() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sync
}

func (w *WalletBackend) setSync(sync bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sync = sync
}
