package backend

import (
	"context"
	"crypto"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/ainvaltin/httpsrv"
	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"

	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/logger"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
)

type (
	WalletBackendService interface {
		GetBills(pubKey []byte, includeDCBills bool, offsetKey []byte, limit int) ([]*Bill, []byte, error)
		GetBill(unitID []byte) (*Bill, error)
		GetFeeCreditBill(unitID []byte) (*Bill, error)
		GetLockedFeeCredit(systemID, fcbID []byte) (*types.TransactionRecord, error)
		GetClosedFeeCredit(fcbID []byte) (*types.TransactionRecord, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		SendTransactions(ctx context.Context, txs []*types.TransactionOrder) map[string]string
		GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)
		HandleTransactionsSubmission(egp *errgroup.Group, sender sdk.PubKey, txs []*types.TransactionOrder)
		GetTxHistoryRecords(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
	}

	ABClient interface {
		SendTransaction(ctx context.Context, tx *types.TransactionOrder) error
		GetRoundNumber(ctx context.Context) (uint64, error)
	}

	WalletBackend struct {
		store BillStore
		abc   ABClient
	}

	Bills struct {
		Bills []*Bill `json:"bills"`
	}

	Bill struct {
		Id                   []byte `json:"id"`
		Value                uint64 `json:"value"`
		TxHash               []byte `json:"txHash"`
		DCTargetUnitID       []byte `json:"dcTargetUnitId,omitempty"`
		DCTargetUnitBacklink []byte `json:"dcTargetUnitBacklink,omitempty"`
		OwnerPredicate       []byte `json:"ownerPredicate"`
		Locked               uint64 `json:"locked"`
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
		GetBills(ownerCondition []byte, includeDCBills bool, offsetKey []byte, limit int) ([]*Bill, []byte, error)
		SetBill(bill *Bill, proof *sdk.Proof) error
		RemoveBill(unitID []byte) error
		SetBillExpirationTime(blockNumber uint64, unitID []byte) error
		DeleteExpiredBills(blockNumber uint64) error
		GetFeeCreditBill(unitID []byte) (*Bill, error)
		SetFeeCreditBill(fcb *Bill, proof *sdk.Proof) error
		GetLockedFeeCredit(systemID, fcbID []byte) (*types.TransactionRecord, error)
		SetLockedFeeCredit(systemID, fcbID []byte, txr *types.TransactionRecord) error
		GetClosedFeeCredit(unitID []byte) (*types.TransactionRecord, error)
		SetClosedFeeCredit(unitID []byte, txr *types.TransactionRecord) error
		GetSystemDescriptionRecords() ([]*genesis.SystemDescriptionRecord, error)
		SetSystemDescriptionRecords(sdrs []*genesis.SystemDescriptionRecord) error
		GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)
		StoreTxHistoryRecord(hash sdk.PubKeyHash, rec *sdk.TxHistoryRecord) error
		GetTxHistoryRecords(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
		StoreTxProof(unitID types.UnitID, txHash sdk.TxHash, txProof *sdk.Proof) error
	}

	p2pkhOwnerPredicates struct {
		sha256 []byte
	}

	Config struct {
		ABMoneySystemIdentifier  []byte
		AlphabillUrl             string
		ServerAddr               string
		DbFile                   string
		ListBillsPageLimit       int
		InitialBill              InitialBill
		SystemDescriptionRecords []*genesis.SystemDescriptionRecord
		Logger                   *slog.Logger
	}

	InitialBill struct {
		Id        []byte
		Value     uint64
		Predicate []byte
	}
)

func Run(ctx context.Context, config *Config) error {
	if config.Logger != nil {
		config.Logger.Info(fmt.Sprintf("starting money backend: BuildInfo=%s", debug.ReadBuildInfo()))
	}
	store, err := newBoltBillStore(config.DbFile)
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
		}, nil)
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
			}, nil)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	abc, err := client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl}, config.Logger)
	if err != nil {
		return err
	}
	defer func() {
		if err := abc.Close(); err != nil {
			config.Logger.Warn("closing AB client", logger.Error(err))
		}
	}()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		config.Logger.Info(fmt.Sprintf("money backend REST server starting on %s", config.ServerAddr))

		handler := &moneyRestAPI{
			Service:            &WalletBackend{store: store, abc: abc},
			ListBillsPageLimit: config.ListBillsPageLimit,
			SystemID:           config.ABMoneySystemIdentifier,
			rw:                 &sdk.ResponseWriter{LogErr: func(err error) { config.Logger.Error(err.Error()) }},
		}

		return httpsrv.Run(ctx,
			http.Server{
				Addr:              config.ServerAddr,
				Handler:           handler.Router(),
				ReadTimeout:       3 * time.Second,
				ReadHeaderTimeout: time.Second,
				WriteTimeout:      5 * time.Second,
				IdleTimeout:       30 * time.Second,
			},
			httpsrv.ShutdownTimeout(5*time.Second),
		)
	})

	g.Go(func() error {
		blockProcessor, err := NewBlockProcessor(store, config.ABMoneySystemIdentifier, config.Logger)
		if err != nil {
			return fmt.Errorf("failed to create block processor: %w", err)
		}
		getBlockNumber := func() (uint64, error) { return store.Do().GetBlockNumber() }
		// we act as if all errors returned by block sync are recoverable ie we
		// just retry in a loop until ctx is cancelled
		for {
			config.Logger.DebugContext(ctx, "starting block sync")
			err := runBlockSync(ctx, abc.GetBlocks, getBlockNumber, 100, blockProcessor.ProcessBlock)
			if err != nil {
				config.Logger.Error("synchronizing blocks returned error", logger.Error(err))
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

// GetBills returns first N=limit bills for given owner predicate starting from the offsetKey
// or if offsetKey is nil then starting from the very first key.
// Always returns the next key if it exists i.e. even if limit=0.
// Furthermore, the next key might not match the filter (isDCBill).
func (w *WalletBackend) GetBills(pubkey []byte, includeDCBills bool, offsetKey []byte, limit int) ([]*Bill, []byte, error) {
	keyHashes := account.NewKeyHash(pubkey)
	ownerPredicates := newOwnerPredicates(keyHashes)
	nextKey := offsetKey
	bills := make([]*Bill, 0, limit)
	for _, predicate := range [][]byte{ownerPredicates.sha256} {
		remainingLimit := limit - len(bills)
		batch, batchNextKey, err := w.store.Do().GetBills(predicate, includeDCBills, nextKey, remainingLimit)
		if err != nil {
			return nil, nil, err
		}
		bills = append(bills, batch...)

		nextKey = batchNextKey
		if nextKey != nil {
			break // more bills in the same predicate batch; return response immediately
		}
		// no more bills in this predicate; move to the next predicate to load more bills
	}
	return bills, nextKey, nil
}

// GetBill returns most recently seen bill with given unit id.
func (w *WalletBackend) GetBill(unitID []byte) (*Bill, error) {
	return w.store.Do().GetBill(unitID)
}

func (w *WalletBackend) GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	return w.store.Do().GetTxProof(unitID, txHash)
}

// GetFeeCreditBill returns most recently seen fee credit bill with given unit id.
func (w *WalletBackend) GetFeeCreditBill(unitID []byte) (*Bill, error) {
	return w.store.Do().GetFeeCreditBill(unitID)
}

// GetLockedFeeCredit returns most recently seen transferFC transaction for given system ID and fee credit bill ID.
func (w *WalletBackend) GetLockedFeeCredit(systemID, fcbID []byte) (*types.TransactionRecord, error) {
	return w.store.Do().GetLockedFeeCredit(systemID, fcbID)
}

// GetClosedFeeCredit returns most recently seen closeFC transaction for given fee credit bill ID.
func (w *WalletBackend) GetClosedFeeCredit(fcbID []byte) (*types.TransactionRecord, error) {
	return w.store.Do().GetClosedFeeCredit(fcbID)
}

// GetRoundNumber returns latest round number.
func (w *WalletBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	return w.abc.GetRoundNumber(ctx)
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
			if err := w.abc.SendTransaction(ctx, tx); err != nil {
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

func (w *WalletBackend) GetTxHistoryRecords(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
	return w.store.Do().GetTxHistoryRecords(hash, dbStartKey, count)
}

func (w *WalletBackend) HandleTransactionsSubmission(egp *errgroup.Group, sender sdk.PubKey, txs []*types.TransactionOrder) {
	egp.Go(func() error { return w.storeIncomingTransactions(sender, txs) })
}

func (w *WalletBackend) storeIncomingTransactions(sender sdk.PubKey, txs []*types.TransactionOrder) error {
	for _, tx := range txs {
		var newOwners []sdk.Predicate
		switch tx.PayloadType() {
		case money.PayloadTypeTransfer:
			attrs := &money.TransferAttributes{}
			err := tx.UnmarshalAttributes(attrs)
			if err != nil {
				return err
			}
			newOwners = append(newOwners, attrs.NewBearer)
		case money.PayloadTypeSplit:
			attrs := &money.SplitAttributes{}
			err := tx.UnmarshalAttributes(attrs)
			if err != nil {
				return err
			}
			for _, targetUnit := range attrs.TargetUnits {
				newOwners = append(newOwners, targetUnit.OwnerCondition)
			}
		}

		for _, newOwner := range newOwners {
			rec := &sdk.TxHistoryRecord{
				UnitID:       tx.UnitID(),
				TxHash:       tx.Hash(crypto.SHA256),
				Timeout:      tx.Timeout(),
				State:        sdk.UNCONFIRMED,
				Kind:         sdk.OUTGOING,
				CounterParty: extractOwnerHashFromP2pkh(newOwner),
			}
			if err := w.store.Do().StoreTxHistoryRecord(sender.Hash(), rec); err != nil {
				return fmt.Errorf("failed to store tx history record: %w", err)
			}
		}
	}
	return nil
}

// extractOwnerFromP2pkh extracts owner from p2pkh predicate.
func extractOwnerHashFromP2pkh(bearer sdk.Predicate) sdk.PubKeyHash {
	pkh, _ := templates.ExtractPubKeyHash(bearer)
	return pkh
}

func extractOwnerKeyFromProof(signature sdk.Predicate) sdk.PubKey {
	pk, _ := templates.ExtractPubKey(signature)
	return pk
}

func (b *Bill) ToGenericBill() *sdk.Bill {
	return &sdk.Bill{
		Id:                   b.Id,
		Value:                b.Value,
		TxHash:               b.TxHash,
		DCTargetUnitID:       b.DCTargetUnitID,
		DCTargetUnitBacklink: b.DCTargetUnitBacklink,
		Locked:               b.Locked,
	}
}

func (b *Bill) ToGenericBills() *sdk.Bills {
	return &sdk.Bills{
		Bills: []*sdk.Bill{
			b.ToGenericBill(),
		},
	}
}

func (b *Bill) getValue() uint64 {
	if b != nil {
		return b.Value
	}
	return 0
}

func (b *Bill) IsDCBill() bool {
	if b != nil {
		return len(b.DCTargetUnitID) > 0
	}
	return false
}

func newOwnerPredicates(hashes *account.KeyHashes) *p2pkhOwnerPredicates {
	return &p2pkhOwnerPredicates{sha256: templates.NewP2pkh256BytesFromKeyHash(hashes.Sha256)}
}
