package pubkey_indexer

import (
	"context"
	"errors"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	moneytx "github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	txverifier "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_verifier"
)

var (
	errKeyNotIndexed  = errors.New("pubkey is not indexed")
	errBillsIsNil     = errors.New("bills input is empty")
	errEmptyBillsList = errors.New("bills list is empty")
)

type (
	WalletBackend struct {
		store         BillStore
		genericWallet *wallet.Wallet
		txConverter   TxConverter
		verifiers     map[string]abcrypto.Verifier
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
		OrderNumber uint64   `json:"orderNumber"`
		TxProof     *TxProof `json:"txProof"`
	}

	TxProof struct {
		BlockNumber uint64                `json:"blockNumber"`
		Tx          *txsystem.Transaction `json:"tx"`
		Proof       *block.BlockProof     `json:"proof"`
	}

	Pubkey struct {
		Pubkey     []byte             `json:"pubkey"`
		PubkeyHash *account.KeyHashes `json:"pubkeyHash"`
	}

	BillStore interface {
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error
		GetBills(pubKey []byte) ([]*Bill, error)
		RemoveBill(pubKey []byte, id []byte) error
		ContainsBill(pubkey []byte, unitID []byte) (bool, error)
		GetBill(pubkey []byte, unitID []byte) (*Bill, error)
		SetBills(pubkey []byte, bills ...*Bill) error
		SetBillExpirationTime(blockNumber uint64, pubkey []byte, unitID []byte) error
		DeleteExpiredBills(blockNumber uint64) error
		GetKeys() ([]*Pubkey, error)
		GetKey(pubkey []byte) (*Pubkey, error)
		AddKey(key *Pubkey) error
	}

	TxConverter interface {
		ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)
	}
)

// New creates a new wallet backend service which can be started by calling the Start or StartProcess method.
// Shutdown method should be called to close resources used by the service.
func New(wallet *wallet.Wallet, store BillStore, txConverter TxConverter, verifiers map[string]abcrypto.Verifier) *WalletBackend {
	return &WalletBackend{
		store:         store,
		genericWallet: wallet,
		txConverter:   txConverter,
		verifiers:     verifiers,
		cancelSyncCh:  make(chan bool, 1),
	}
}

// NewPubkey creates a new hashed Pubkey
func NewPubkey(pubkey []byte) *Pubkey {
	return &Pubkey{
		Pubkey:     pubkey,
		PubkeyHash: account.NewKeyHash(pubkey),
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

func (w *WalletBackend) SystemID() []byte {
	// TODO: return the default "AlphaBill Money System ID" for now
	// but this should come from config (w.genericWallet?)
	return []byte{0, 0, 0, 0}
}

// GetBills returns all bills for given public key.
func (w *WalletBackend) GetBills(pubkey []byte) ([]*Bill, error) {
	return w.store.GetBills(pubkey)
}

// GetBill returns most recently seen bill with given unit id.
func (w *WalletBackend) GetBill(pubkey []byte, unitID []byte) (*Bill, error) {
	return w.store.GetBill(pubkey, unitID)
}

// SetBill adds new bill to the index.
// Bill most have a valid block proof.
// Overwrites existing bill, if one exists.
// Returns error if given pubkey is not indexed.
func (w *WalletBackend) SetBills(pubkey []byte, bills *moneytx.Bills) error {
	if bills == nil {
		return errBillsIsNil
	}
	if len(bills.Bills) == 0 {
		return errEmptyBillsList
	}
	err := bills.Verify(w.txConverter, w.verifiers)
	if err != nil {
		return err
	}
	key, err := w.store.GetKey(pubkey)
	if err != nil {
		return err
	}
	if key == nil {
		return errKeyNotIndexed
	}
	pubkeyHash := account.NewKeyHash(pubkey)
	domainBills := newBillsFromProto(bills)
	for _, bill := range domainBills {
		tx, err := w.txConverter.ConvertTx(bill.TxProof.Tx)
		if err != nil {
			return err
		}
		err = txverifier.VerifyTxP2PKHOwner(tx, pubkeyHash)
		if err != nil {
			return err
		}
	}
	return w.store.SetBills(pubkey, domainBills...)
}

// AddKey adds new public key to list of tracked keys.
// Returns ErrKeyAlreadyExists error if key already exists.
func (w *WalletBackend) AddKey(pubkey []byte) error {
	return w.store.AddKey(NewPubkey(pubkey))
}

// GetMaxBlockNumber returns max block number known to the connected AB node.
func (w *WalletBackend) GetMaxBlockNumber() (uint64, error) {
	blNo, _, err := w.genericWallet.GetMaxBlockNumber() // TODO return latest round number?
	return blNo, err
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

func (b *Bill) toProto() *moneytx.Bill {
	return &moneytx.Bill{
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

func (b *Bill) toProtoBills() *moneytx.Bills {
	return &moneytx.Bills{
		Bills: []*moneytx.Bill{
			b.toProto(),
		},
	}
}

func newBillsFromProto(src *moneytx.Bills) []*Bill {
	dst := make([]*Bill, len(src.Bills))
	for i, b := range src.Bills {
		dst[i] = newBill(b)
	}
	return dst
}

func newBill(b *moneytx.Bill) *Bill {
	return &Bill{
		Id:       b.Id,
		Value:    b.Value,
		TxHash:   b.TxHash,
		IsDCBill: b.IsDcBill,
		TxProof:  newTxProof(b.TxProof),
	}
}

func newTxProof(b *block.TxProof) *TxProof {
	return &TxProof{
		BlockNumber: b.BlockNumber,
		Tx:          b.Tx,
		Proof:       b.Proof,
	}
}
