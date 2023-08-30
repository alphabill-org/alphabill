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
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
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

	WalletBackend struct {
		store         BillStore
		genericWallet *sdk.Wallet
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

		// fcb specific fields
		// LastAddFCTxHash last add fee credit tx hash
		LastAddFCTxHash []byte `json:"lastAddFcTxHash,omitempty"`
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

	abc := client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl})

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		walletBackend := &WalletBackend{store: store, genericWallet: sdk.New().SetABClient(abc).Build()}
		defer walletBackend.genericWallet.Shutdown()

		handler := &moneyRestAPI{Service: walletBackend, ListBillsPageLimit: config.ListBillsPageLimit, rw: &sdk.ResponseWriter{LogErr: wlog.Error}}
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

// GetBills returns first N=limit bills for given owner predicate starting from the offsetKey
// or if offsetKey is nil then starting from the very first key.
// Always returns the next key if it exists i.e. even if limit=0.
// Furthermore, the next key might not match the filter (isDCBill).
func (w *WalletBackend) GetBills(pubkey []byte, includeDCBills bool, offsetKey []byte, limit int) ([]*Bill, []byte, error) {
	keyHashes := account.NewKeyHash(pubkey)
	ownerPredicates := newOwnerPredicates(keyHashes)
	nextKey := offsetKey
	bills := make([]*Bill, 0, limit)
	for _, predicate := range [][]byte{ownerPredicates.sha256, ownerPredicates.sha512} {
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

func (w *WalletBackend) GetTxHistoryRecords(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
	return w.store.Do().GetTxHistoryRecords(hash, dbStartKey, count)
}

func (w *WalletBackend) HandleTransactionsSubmission(egp *errgroup.Group, sender sdk.PubKey, txs []*types.TransactionOrder) {
	egp.Go(func() error { return w.storeIncomingTransactions(sender, txs) })
}

func (w *WalletBackend) storeIncomingTransactions(sender sdk.PubKey, txs []*types.TransactionOrder) error {
	for _, tx := range txs {
		var newOwner sdk.Predicate
		switch tx.PayloadType() {
		case money.PayloadTypeTransfer:
			attrs := &money.TransferAttributes{}
			err := tx.UnmarshalAttributes(attrs)
			if err != nil {
				return err
			}
			newOwner = attrs.NewBearer
		case money.PayloadTypeSplit:
			attrs := &money.SplitAttributes{}
			err := tx.UnmarshalAttributes(attrs)
			if err != nil {
				return err
			}
			newOwner = attrs.TargetBearer
		default:
			continue
		}

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
	return nil
}

// extractOwnerFromP2pkh extracts owner from p2pkh predicate.
func extractOwnerHashFromP2pkh(bearer sdk.Predicate) sdk.PubKeyHash {
	// p2pkh owner predicate must be 10 + (32 or 64) (SHA256 or SHA512) bytes long
	if len(bearer) != 42 && len(bearer) != 74 {
		return nil
	}
	// 6th byte is HashAlgo 0x01 or 0x02 for SHA256 and SHA512 respectively
	hashAlgo := bearer[5]
	if hashAlgo == script.HashAlgSha256 {
		return sdk.PubKeyHash(bearer[6:38])
	} else if hashAlgo == script.HashAlgSha512 {
		return sdk.PubKeyHash(bearer[6:70])
	}
	return nil
}

func extractOwnerKeyFromProof(signature sdk.Predicate) sdk.PubKey {
	if len(signature) == 103 && signature[68] == script.OpPushPubKey && signature[69] == script.SigSchemeSecp256k1 {
		return sdk.PubKey(signature[70:])
	}
	return nil
}

func (b *Bill) ToGenericBill() *sdk.Bill {
	return &sdk.Bill{
		Id:                   b.Id,
		Value:                b.Value,
		TxHash:               b.TxHash,
		DCTargetUnitID:       b.DCTargetUnitID,
		DCTargetUnitBacklink: b.DCTargetUnitBacklink,
		LastAddFCTxHash:      b.LastAddFCTxHash,
	}
}

func (b *Bill) ToGenericBills() *sdk.Bills {
	return &sdk.Bills{
		Bills: []*sdk.Bill{
			b.ToGenericBill(),
		},
	}
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

func (b *Bill) getLastAddFCTxHash() []byte {
	if b != nil {
		return b.LastAddFCTxHash
	}
	return nil
}

func (b *Bill) IsDCBill() bool {
	if b != nil {
		return len(b.DCTargetUnitID) > 0
	}
	return false
}

func newOwnerPredicates(hashes *account.KeyHashes) *p2pkhOwnerPredicates {
	return &p2pkhOwnerPredicates{
		sha256: script.PredicatePayToPublicKeyHash(script.HashAlgSha256, hashes.Sha256, script.SigSchemeSecp256k1),
		sha512: script.PredicatePayToPublicKeyHash(script.HashAlgSha512, hashes.Sha512, script.SigSchemeSecp256k1),
	}
}
