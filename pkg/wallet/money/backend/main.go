package backend

import (
	"context"
	"crypto"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/ainvaltin/httpsrv"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

// @title           Money Partition Indexing Backend API
// @version         1.0
// @description     This service processes blocks from the Money partition and indexes ownership of bills.

// @BasePath  /api/v1
type (
	WalletBackendService interface {
		GetBills(ownerCondition []byte) ([]*Bill, error)
		GetBill(unitID []byte) (*Bill, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(unitID []byte) (*Bill, error)
		SendTransactions(ctx context.Context, txs []*types.TransactionOrder) map[string]string
	}

	WalletBackend struct {
		store         BillStore
		genericWallet *wallet.Wallet
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
		OrderNumber    uint64        `json:"orderNumber"`
		TxProof        *wallet.Proof `json:"txProof"`
		OwnerPredicate []byte        `json:"OwnerPredicate"`

		// fcb specific fields
		// FCBlockNumber block number when fee credit bill balance was last updated
		FCBlockNumber uint64 `json:"fcBlockNumber"`
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
		GetSystemDescriptionRecords() ([]*genesis.SystemDescriptionRecord, error)
		SetSystemDescriptionRecords(sdrs []*genesis.SystemDescriptionRecord) error
	}

	p2pkhOwnerPredicates struct {
		sha256 []byte
		sha512 []byte
	}

	Config struct {
		ABMoneySystemIdentifier  []byte
		AlphabillUrl             string
		ServerAddr               string
		DbFile                   string
		ListBillsPageLimit       int
		InitialBill              InitialBill
		SystemDescriptionRecords []*genesis.SystemDescriptionRecord
	}

	InitialBill struct {
		Id        []byte
		Value     uint64
		Predicate []byte
	}
)

func Run(ctx context.Context, config *Config) error {
	store, err := NewBoltBillStore(config.DbFile)
	if err != nil {
		return fmt.Errorf("failed to get storage: %w", err)
	}

	// if first run:
	// store initial bill to avoid some edge cases
	// store system description records and partition fee bills to avoid providing them every run
	err = store.WithTransaction(func(txc BillStoreTx) error {
		blockNumber, err := txc.GetBlockNumber()
		if err != nil {
			return err
		}
		if blockNumber > 0 {
			return nil
		}
		ib := config.InitialBill
		err = txc.SetBill(&Bill{
			Id:             ib.Id,
			Value:          ib.Value,
			OwnerPredicate: ib.Predicate,
		})
		if err != nil {
			return fmt.Errorf("failed to store initial bill: %w", err)
		}

		err = txc.SetSystemDescriptionRecords(config.SystemDescriptionRecords)
		if err != nil {
			return fmt.Errorf("failed to store system description records: %w", err)
		}
		for _, sdr := range config.SystemDescriptionRecords {
			err = txc.SetBill(&Bill{
				Id:             sdr.FeeCreditBill.UnitId,
				OwnerPredicate: sdr.FeeCreditBill.OwnerPredicate,
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	abc := client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl})

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		walletBackend := &WalletBackend{store: store, genericWallet: wallet.New().SetABClient(abc).Build()}
		defer walletBackend.genericWallet.Shutdown()

		handler := &RequestHandler{Service: walletBackend, ListBillsPageLimit: config.ListBillsPageLimit}
		server := http.Server{
			Addr:              config.ServerAddr,
			Handler:           handler.Router(),
			ReadTimeout:       3 * time.Second,
			ReadHeaderTimeout: time.Second,
			WriteTimeout:      5 * time.Second,
			IdleTimeout:       30 * time.Second,
		}

		return httpsrv.Run(ctx, server, httpsrv.ShutdownTimeout(5*time.Second))
	})

	g.Go(func() error {
		blockProcessor, err := NewBlockProcessor(store, config.ABMoneySystemIdentifier)
		if err != nil {
			return fmt.Errorf("failed to create block processor: %w", err)
		}
		getBlockNumber := func() (uint64, error) { return store.Do().GetBlockNumber() }
		// we act as if all errors returned by block sync are recoverable ie we
		// just retry in a loop until ctx is cancelled
		for {
			wlog.Debug("starting block sync")
			err := runBlockSync(ctx, abc.GetBlocks, getBlockNumber, 100, blockProcessor.ProcessBlock)
			if err != nil {
				wlog.Error("synchronizing blocks returned error: ", err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(rand.Int31n(10)+10) * time.Second):
			}
		}
	})

	return g.Wait()
}

func runBlockSync(ctx context.Context, getBlocks blocksync.BlocksLoaderFunc, getBlockNumber func() (uint64, error), batchSize int, processor blocksync.BlockProcessorFunc) error {
	blockNumber, err := getBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to read current block number for a sync starting point: %w", err)
	}
	// on bootstrap storage returns 0 as current block and as block numbering
	// starts from 1 by adding 1 to it we start with the first block
	return blocksync.Run(ctx, getBlocks, blockNumber+1, 0, batchSize, processor)
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

// GetRoundNumber returns latest round number.
func (w *WalletBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.genericWallet.GetRoundNumber(ctx)
}

// TODO: Share functionaly with tokens partiton
// SendTransactions forwards transactions to partiton node(s).
func (w *WalletBackend) SendTransactions(ctx context.Context, txs []*types.TransactionOrder) map[string]string {
	errs := make(map[string]string)
	var m sync.Mutex

	const maxWorkers = 5
	sem := semaphore.NewWeighted(maxWorkers)
	for _, tx := range txs {
		if err := sem.Acquire(ctx, 1); err != nil {
			break
		}
		go func(tx *types.TransactionOrder) {
			defer sem.Release(1)
			if err := w.genericWallet.SendTransaction(ctx, tx, nil); err != nil {
				m.Lock()
				errs[hex.EncodeToString(tx.UnitID())] =
					fmt.Errorf("failed to forward tx: %w", err).Error()
				m.Unlock()
			}
		}(tx)
	}

	semCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := sem.Acquire(semCtx, maxWorkers); err != nil {
		m.Lock()
		errs["waiting-for-workers"] = err.Error()
		m.Unlock()
	}
	return errs
}

func (b *Bill) toProto() *wallet.Bill {
	return &wallet.Bill{
		Id:            b.Id,
		Value:         b.Value,
		TxHash:        b.TxHash,
		IsDcBill:      b.IsDCBill,
		TxProof:       b.TxProof,
		FcBlockNumber: b.FCBlockNumber,
	}
}

func (b *Bill) toProtoBills() *wallet.Bills {
	return &wallet.Bills{
		Bills: []*wallet.Bill{
			b.toProto(),
		},
	}
}

func (b *Bill) addProof(txIdx int, bl *types.Block) error {
	proof, err := wallet.NewTxProof(txIdx, bl, crypto.SHA256)
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

func newOwnerPredicates(hashes *account.KeyHashes) *p2pkhOwnerPredicates {
	return &p2pkhOwnerPredicates{
		sha256: script.PredicatePayToPublicKeyHashDefault(hashes.Sha256),
		sha512: script.PredicatePayToPublicKeyHashDefault(hashes.Sha512),
	}
}
