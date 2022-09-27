package backend

import (
	"context"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
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

	Bill struct {
		Id    *uint256.Int
		Value uint64
	}

	BlockProof struct {
		BillId      *uint256.Int      `json:"billId"`
		BlockNumber uint64            `json:"blockNumber"`
		BlockProof  *block.BlockProof `json:"blockProof"`
	}

	Pubkey struct {
		Pubkey     []byte            `json:"pubkey"`
		PubkeyHash *wallet.KeyHashes `json:"pubkeyHash"`
	}

	BillStore interface {
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error
		GetBills(pubKey []byte) ([]*Bill, error)
		AddBill(pubKey []byte, bill *Bill) error
		AddBillWithProof(pubKey []byte, bill *Bill, proof *BlockProof) error
		RemoveBill(pubKey []byte, id *uint256.Int) error
		ContainsBill(pubKey []byte, id *uint256.Int) (bool, error)
		GetBlockProof(billId []byte) (*BlockProof, error)
		SetBlockProof(proof *BlockProof) error
		GetKeys() ([]*Pubkey, error)
		AddKey(key *Pubkey) error
	}
)

// New creates a new wallet backend service which can be started by calling the Start or StartProcess method.
// Shutdown method should be called to close resources used by the service.
func New(wallet *wallet.Wallet, store BillStore) *WalletBackend {
	return &WalletBackend{store: store, genericWallet: wallet, cancelSyncCh: make(chan bool, 1)}
}

// NewPubkey creates a new hashed Pubkey
func NewPubkey(pubkey []byte) *Pubkey {
	return &Pubkey{
		Pubkey:     pubkey,
		PubkeyHash: wallet.NewKeyHash(pubkey),
	}
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
func (w *WalletBackend) GetBills(pubkey []byte) ([]*Bill, error) {
	return w.store.GetBills(pubkey)
}

// GetBlockProof returns most recent proof for given unit id.
func (w *WalletBackend) GetBlockProof(unitId []byte) (*BlockProof, error) {
	return w.store.GetBlockProof(unitId)
}

// AddKey adds new public key to list of tracked keys.
func (w *WalletBackend) AddKey(pubkey []byte) error {
	return w.store.AddKey(NewPubkey(pubkey))
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
