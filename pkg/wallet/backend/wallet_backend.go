package backend

import (
	"context"
	"crypto"
	"errors"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/schema"
)

var alphabillMoneySystemId = []byte{0, 0, 0, 0}

type (
	WalletBackend struct {
		store         BillStore
		genericWallet *wallet.Wallet
		verifiers     map[string]abcrypto.Verifier
		cancelSyncCh  chan bool
	}

	Bill struct {
		Id     []byte `json:"id" validate:"required,len=32"`
		Value  uint64 `json:"value" validate:"required"`
		TxHash []byte `json:"txHash" validate:"required"`
		// OrderNumber insertion order of given bill in pubkey => list of bills bucket, needed for determistic paging
		OrderNumber uint64      `json:"orderNumber"`
		BlockProof  *BlockProof `json:"blockProof" validate:"required"`
	}

	BlockProof struct {
		BlockNumber uint64                `json:"blockNumber" validate:"required"`
		Tx          *txsystem.Transaction `json:"tx" validate:"required"`
		Proof       *block.BlockProof     `json:"proof" validate:"required"`
	}

	Pubkey struct {
		Pubkey     []byte            `json:"pubkey"`
		PubkeyHash *wallet.KeyHashes `json:"pubkeyHash"`
	}

	BillStore interface {
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error
		GetBills(pubKey []byte) ([]*Bill, error)
		RemoveBill(pubKey []byte, id []byte) error
		ContainsBill(id []byte) (bool, error)
		GetBill(billId []byte) (*Bill, error)
		SetBill(pubkey []byte, bill *Bill) error
		GetKeys() ([]*Pubkey, error)
		AddKey(key *Pubkey) error
	}
)

// New creates a new wallet backend service which can be started by calling the Start or StartProcess method.
// Shutdown method should be called to close resources used by the service.
func New(wallet *wallet.Wallet, store BillStore, verifiers map[string]abcrypto.Verifier) *WalletBackend {
	return &WalletBackend{store: store, genericWallet: wallet, verifiers: verifiers, cancelSyncCh: make(chan bool, 1)}
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

// GetBill returns most recently seen bill with given unit id.
func (w *WalletBackend) GetBill(unitId []byte) (*Bill, error) {
	return w.store.GetBill(unitId)
}

// SetBill adds new bill to the index.
// Bill most have a valid block proof.
// Overwrites existing bill, if one exists.
// Returns error if given pubkey is not indexed.
func (w *WalletBackend) SetBill(pubkey []byte, bill *Bill) error {
	// TODO if pubkey is not tracked => return error
	err := bill.BlockProof.verifyProof(w.verifiers)
	if err != nil {
		return err
	}
	return w.store.SetBill(pubkey, bill)
}

// AddKey adds new public key to list of tracked keys.
// Returns ErrKeyAlreadyExists error if key already exists.
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

func (b *BlockProof) verifyProof(verifiers map[string]abcrypto.Verifier) error {
	if b.Proof == nil {
		return errors.New("proof is nil")
	}
	gtx, err := txConverter.ConvertTx(b.Tx)
	if err != nil {
		return err
	}
	return b.Proof.Verify(gtx, verifiers, crypto.SHA256)
}

func (b *Bill) toSchema() *schema.Bill {
	return &schema.Bill{
		Id:         b.Id,
		Value:      b.Value,
		TxHash:     b.TxHash,
		BlockProof: b.BlockProof.toSchema(),
	}
}

func (b *BlockProof) toSchema() *schema.BlockProof {
	return &schema.BlockProof{
		BlockNumber: b.BlockNumber,
		Tx:          b.Tx,
		Proof:       b.Proof,
	}
}
