package money

import (
	"context"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type (
	WalletBackend struct {
		store         BillStore
		genericWallet *wallet.Wallet
		cancelSyncCh  chan bool
	}

	Bills struct {
		Bills []*Bill `json:"bills"`
	}

	Bill struct {
		Id       []byte `json:"id"`
		Value    uint64 `json:"value"`
		TxHash   []byte `json:"txHash"`
		IsDCBill bool   `json:"isDcBill"`
		// OrderNumber insertion order of given bill in pubkey => list of bills bucket, needed for determistic paging
		OrderNumber    uint64   `json:"orderNumber"`
		TxProof        *TxProof `json:"txProof"`
		OwnerPredicate []byte   `json:"OwnerPredicate"`
	}

	TxProof struct {
		BlockNumber uint64                `json:"blockNumber"`
		Tx          *txsystem.Transaction `json:"tx"`
		Proof       *block.BlockProof     `json:"proof"`
	}

	Pubkey struct {
		Pubkey     []byte            `json:"pubkey"`
		PubkeyHash *wallet.KeyHashes `json:"pubkeyHash"`
	}

	// BillStore type for creating BillStoreTx transactions
	BillStore interface {
		Do() BillStoreTx
		WithTransaction(func(tx BillStoreTx) error) error
	}

	// BillStoreTx type for managing units by their ID and owner condition
	BillStoreTx interface {
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error
		GetBill(unitID []byte) (*Bill, error)
		GetBills(ownerCondition []byte) ([]*Bill, error)
		SetBill(bill *Bill) error
		RemoveBill(unitID []byte) error
		SetBillExpirationTime(blockNumber uint64, unitID []byte) error
		DeleteExpiredBills(blockNumber uint64) error
	}

	p2pkhOwnerPredicates struct {
		sha256 []byte
		sha512 []byte
	}
)

// New creates a new wallet backend Service which can be started by calling the Start or StartProcess method.
// Shutdown method should be called to close resources used by the Service.
func New(wallet *wallet.Wallet, store BillStore) *WalletBackend {
	return &WalletBackend{store: store, genericWallet: wallet, cancelSyncCh: make(chan bool, 1)}
}

// Start starts downloading blocks and indexing bills by their owner's public key.
// Blocks forever or until alphabill connection is terminated.
func (w *WalletBackend) Start(ctx context.Context) error {
	blockNumber, err := w.store.Do().GetBlockNumber()
	if err != nil {
		return err
	}
	return w.genericWallet.Sync(ctx, blockNumber)
}

// StartProcess calls Start in a retry loop, can be canceled by cancelling context or calling Shutdown method.
func (w *WalletBackend) StartProcess(ctx context.Context) {
	wlog.Info("starting wallet-backend synchronization")
	defer wlog.Info("wallet-backend synchronization ended")
	for {
		if err := w.Start(ctx); err != nil {
			wlog.Error("error synchronizing wallet-backend: ", err)
		}
		// delay before retrying
		select {
		case <-ctx.Done(): // canceled from context
			return
		case <-w.cancelSyncCh: // canceled from shutdown method
			return
		case <-time.After(10 * time.Second):
		}
	}
}

// GetBills returns all bills for given public key.
func (w *WalletBackend) GetBills(pubkey []byte) ([]*Bill, error) {
	keyHashes := wallet.NewKeyHash(pubkey)
	ownerPredicates := newOwnerPredicates(keyHashes)
	s1, err := w.store.Do().GetBills(ownerPredicates.sha256)
	if err != nil {
		return nil, err
	}
	s2, err := w.store.Do().GetBills(ownerPredicates.sha512)
	if err != nil {
		return nil, err
	}
	s3 := append(s1, s2...)
	return s3, nil
}

// GetBill returns most recently seen bill with given unit id.
func (w *WalletBackend) GetBill(unitID []byte) (*Bill, error) {
	return w.store.Do().GetBill(unitID)
}

// GetMaxBlockNumber returns max block number known to the connected AB node.
func (w *WalletBackend) GetMaxBlockNumber() (uint64, error) {
	return w.genericWallet.GetMaxBlockNumber()
}

// Shutdown terminates wallet backend Service.
func (w *WalletBackend) Shutdown() {
	// send signal to cancel channel if channel is not full
	select {
	case w.cancelSyncCh <- true:
	default:
	}
	w.genericWallet.Shutdown()
}

func (b *Bill) toProto() *block.Bill {
	return &block.Bill{
		Id:       b.Id,
		Value:    b.Value,
		TxHash:   b.TxHash,
		IsDcBill: b.IsDCBill,
		TxProof:  b.TxProof.toProto(),
	}
}

func (b *TxProof) toProto() *block.TxProof {
	return &block.TxProof{
		BlockNumber: b.BlockNumber,
		Tx:          b.Tx,
		Proof:       b.Proof,
	}
}

func (b *Bill) toProtoBills() *block.Bills {
	return &block.Bills{
		Bills: []*block.Bill{
			b.toProto(),
		},
	}
}

func newOwnerPredicates(hashes *wallet.KeyHashes) *p2pkhOwnerPredicates {
	return &p2pkhOwnerPredicates{
		sha256: script.PredicatePayToPublicKeyHashDefault(hashes.Sha256),
		sha512: script.PredicatePayToPublicKeyHashDefault(hashes.Sha512),
	}
}
