package backend

import (
	"context"
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
		cancelSyncCh  chan bool
	}

	bill struct {
		Id    *uint256.Int
		Value uint64
	}

	blockProof struct {
		BillId      *uint256.Int      `json:"billId"`
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
		AddBillWithProof(pubKey []byte, bill *bill, proof *blockProof) error
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
	return &WalletBackend{store: store, genericWallet: genericWallet, cancelSyncCh: make(chan bool, 1)}
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

// StartProcess calls Start in a retry loop, can be canceled by cancelling context or calling Shutdown method.
func (w *WalletBackend) StartProcess(ctx context.Context) {
	wlog.Info("starting wallet-backend synchronization")
	defer wlog.Info("wallet-backend synchronization ended")
	retryCount := 0
	for {
		select {
		case <-ctx.Done(): // canceled from context
			return
		case <-w.cancelSyncCh: // canceled from shutdown method
			return
		default:
			if retryCount > 0 {
				wlog.Info("sleeping 10s before retrying alphabill connection")
				time.Sleep(10 * time.Second)
			}
			err := w.Start(ctx)
			if err != nil {
				wlog.Error("error synchronizing wallet-backend: ", err)
			}
			retryCount++
		}
	}
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
	// send signal to cancel channel if channel is not full
	select {
	case w.cancelSyncCh <- true:
	default:
	}
	w.genericWallet.Shutdown()
}
