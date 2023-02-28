package money

import (
	"context"
	"crypto"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
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

		// fcb specific fields
		// FCBlockNumber block number when fee credit bill balance was last updated
		FCBlockNumber uint64 `json:"fcBlockNumber"`
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
		GetFeeCreditBill(unitID []byte) (*Bill, error)
		SetFeeCreditBill(fcb *Bill) error
	}

	p2pkhOwnerPredicates struct {
		sha256 []byte
		sha512 []byte
	}

	Config struct {
		ABMoneySystemIdentifier []byte
		AlphabillUrl            string
		ServerAddr              string
		DbFile                  string
		ListBillsPageLimit      int
		InitialBill             InitialBill
	}

	InitialBill struct {
		Id        []byte
		Value     uint64
		Predicate []byte
	}
)

func CreateAndRun(ctx context.Context, config *Config) error {
	store, err := NewBoltBillStore(config.DbFile)
	if err != nil {
		return err
	}

	// store initial bill if first run to avoid some edge cases
	err = store.WithTransaction(func(txc BillStoreTx) error {
		blockNumber, err := txc.GetBlockNumber()
		if err != nil {
			return err
		}
		if blockNumber > 0 {
			return nil
		}
		ib := config.InitialBill
		return txc.SetBill(&Bill{
			Id:             ib.Id,
			Value:          ib.Value,
			OwnerPredicate: ib.Predicate,
		})
	})
	if err != nil {
		return err
	}

	bp := NewBlockProcessor(store, backend.NewTxConverter(config.ABMoneySystemIdentifier))
	w := wallet.New().SetBlockProcessor(bp).SetABClient(client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl})).Build()

	service := New(w, store)
	wg := sync.WaitGroup{}
	if config.AlphabillUrl != "" {
		wg.Add(1)
		go func() {
			service.StartProcess(ctx)
			wg.Done()
		}()
	}

	server := NewHttpServer(config.ServerAddr, config.ListBillsPageLimit, service)
	err = server.Start()
	if err != nil {
		service.Shutdown()
		return aberrors.Wrap(err, "error starting wallet backend http server")
	}

	// listen for termination signal and shutdown the app
	hook := func(sig os.Signal) {
		wlog.Info("Received signal '", sig, "' shutting down application...")
		err := server.Shutdown(context.Background())
		if err != nil {
			wlog.Error("error shutting down server: ", err)
		}
		service.Shutdown()
	}
	listen(hook, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGINT)

	wg.Wait() // wait for service shutdown to complete

	return nil
}

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
	keyHashes := account.NewKeyHash(pubkey)
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

// GetFeeCreditBill returns most recently seen fee credit bill with given unit id.
func (w *WalletBackend) GetFeeCreditBill(unitID []byte) (*Bill, error) {
	return w.store.Do().GetFeeCreditBill(unitID)
}

// GetMaxBlockNumber returns max block number known to the connected AB node.
func (w *WalletBackend) GetMaxBlockNumber() (uint64, uint64, error) {
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

func (b *Bill) toProto() *bp.Bill {
	return &bp.Bill{
		Id:            b.Id,
		Value:         b.Value,
		TxHash:        b.TxHash,
		IsDcBill:      b.IsDCBill,
		TxProof:       b.TxProof.toProto(),
		FcBlockNumber: b.FCBlockNumber,
	}
}

func (b *Bill) toProtoBills() *bp.Bills {
	return &bp.Bills{
		Bills: []*bp.Bill{
			b.toProto(),
		},
	}
}

func (b *Bill) addProof(bl *block.GenericBlock, txPb *txsystem.Transaction) error {
	proof, err := createProof(b.Id, txPb, bl, crypto.SHA256)
	if err != nil {
		return err
	}
	b.TxProof = proof
	return nil
}

func (b *Bill) getTxHash() []byte {
	if b != nil {
		return b.TxHash
	}
	return nil
}

func (b *Bill) getValue() uint64 {
	if b != nil {
		return b.Value
	}
	return 0
}

func (b *Bill) getFCBlockNumber() uint64 {
	if b != nil {
		return b.FCBlockNumber
	}
	return 0
}

func (b *TxProof) toProto() *block.TxProof {
	return &block.TxProof{
		BlockNumber: b.BlockNumber,
		Tx:          b.Tx,
		Proof:       b.Proof,
	}
}

func createProof(unitID []byte, tx *txsystem.Transaction, b *block.GenericBlock, hashAlgorithm crypto.Hash) (*TxProof, error) {
	proof, err := block.NewPrimaryProof(b, unitID, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return newTxProof(tx, proof, b.GetRoundNumber())
}

func newTxProof(tx *txsystem.Transaction, proof *block.BlockProof, blockNumber uint64) (*TxProof, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if proof == nil {
		return nil, errors.New("proof is nil")
	}
	return &TxProof{
		Tx:          tx,
		Proof:       proof,
		BlockNumber: blockNumber,
	}, nil
}

func newOwnerPredicates(hashes *account.KeyHashes) *p2pkhOwnerPredicates {
	return &p2pkhOwnerPredicates{
		sha256: script.PredicatePayToPublicKeyHashDefault(hashes.Sha256),
		sha512: script.PredicatePayToPublicKeyHashDefault(hashes.Sha512),
	}
}

// listen waits for given OS signals and then calls given shutdownHook func
func listen(shutdownHook func(sig os.Signal), signals ...os.Signal) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	sig := <-ch
	shutdownHook(sig)
}
