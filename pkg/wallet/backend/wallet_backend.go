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
		BillId      []byte                `json:"billId" validate:"required,len=32"`
		BlockNumber uint64                `json:"blockNumber" validate:"required,gt=0"`
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
		AddBill(pubKey []byte, bill *Bill) error
		AddBillWithProof(pubKey []byte, bill *Bill, proof *BlockProof) error
		RemoveBill(pubKey []byte, id []byte) error
		ContainsBill(pubKey []byte, id []byte) (bool, error)
		GetBlockProof(billId []byte) (*BlockProof, error)
		SetBlockProof(proof *BlockProof) error
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

// GetBlockProof returns most recent proof for given unit id.
func (w *WalletBackend) GetBlockProof(unitId []byte) (*BlockProof, error) {
	return w.store.GetBlockProof(unitId)
}

// AddBillWithProof adds new bill and proof to the index.
// Verifies block proof.
// Overwrites existing bill and proof, if one exists.
func (w *WalletBackend) AddBillWithProof(pubkey []byte, bill *Bill) error {
	// TODO is pubkey added to tracked keys if not exists??
	err := bill.BlockProof.verifyProof(w.verifiers)
	if err != nil {
		return err
	}
	return w.store.AddBillWithProof(pubkey, bill, bill.BlockProof)
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
