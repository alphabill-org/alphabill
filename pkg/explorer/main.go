package explorer

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/ainvaltin/httpsrv"
	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"golang.org/x/sync/errgroup"
)

type (
	ExplorerBackendService interface {
		GetBlockByBlockNumber(blockNumber uint64) (*types.Block, error)
		GetBlocks(dbStartBlock uint64, count int) (res []*types.Block, prevBlockNumber uint64, err error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)
		GetTxHistoryRecords(dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
		GetTxHistoryRecordsByKey(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
	}

	ExplorerBackend struct {
		store BillStore
		sdk   *sdk.Wallet
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
		GetBlockByBlockNumber(blockNumber uint64) (*types.Block, error)
		GetBlocks(dbStartBlock uint64, count int) (res []*types.Block, prevBlockNumber uint64, err error)
		SetBlock(b *types.Block) error
		GetBlockExplorerByBlockNumber(blockNumber uint64) (*BlockExplorer, error)
		GetBlocksExplorer(dbStartBlock uint64, count int) (res []*BlockExplorer, prevBlockNumber uint64, err error)
		SetBlockExplorer(b *types.Block) error
		GetBlockNumber() (uint64, error)
		SetBlockNumber(blockNumber uint64) error
		GetBill(unitID []byte) (*Bill, error)
		GetBills(ownerCondition []byte, includeDCBills bool, offsetKey []byte, limit int) ([]*Bill, []byte, error)
		SetBill(bill *Bill, proof *sdk.Proof) error
		RemoveBill(unitID []byte) error
		GetSystemDescriptionRecords() ([]*genesis.SystemDescriptionRecord, error)
		SetSystemDescriptionRecords(sdrs []*genesis.SystemDescriptionRecord) error
		GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)
		StoreTxProof(unitID types.UnitID, txHash sdk.TxHash, txProof *sdk.Proof) error
		GetTxHistoryRecords(dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
		GetTxHistoryRecordsByKey(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error)
		StoreTxHistoryRecord(hash sdk.PubKeyHash, rec *sdk.TxHistoryRecord) error
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
		//SystemDescriptionRecords []*genesis.SystemDescriptionRecord
	}

	InitialBill struct {
		ID        []byte
		Value     uint64
		Predicate []byte
	}
)

func Run(ctx context.Context, config *Config) error {
	wlog.Info("starting money backend: BuildInfo=", debug.ReadBuildInfo())
	store, err := newBoltBillStore(config.DbFile)
	if err != nil {
		return fmt.Errorf("failed to get storage: %w", err)
	}

	abc := client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl})

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		wlog.Info("money backend REST server starting on ", config.ServerAddr)
		explorerBackend := &ExplorerBackend{store: store, sdk: sdk.New().SetABClient(abc).Build()}
		defer explorerBackend.sdk.Shutdown()

		handler := &moneyRestAPI{
			Service:            explorerBackend,
			ListBillsPageLimit: config.ListBillsPageLimit,
			SystemID:           config.ABMoneySystemIdentifier,
			rw:                 &sdk.ResponseWriter{LogErr: wlog.Error},
		}
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


// GetBlockByBlockNumber returns block with given block number.
func (ex *ExplorerBackend) GetBlockByBlockNumber(blockNumber uint64) (*types.Block, error) {
	return ex.store.Do().GetBlockByBlockNumber(blockNumber)
}

// GetBlock return amount of blocks provided with count
func (ex *ExplorerBackend) GetBlocks(dbStartBlockNumber uint64, count int) (res []*types.Block, prevBlockNUmber uint64, err error) {
	return ex.store.Do().GetBlocks(dbStartBlockNumber, count)
}

// GetBill returns most recently seen bill with given unit id.
func (ex *ExplorerBackend) GetBill(unitID []byte) (*Bill, error) {
	return ex.store.Do().GetBill(unitID)
}

func (ex *ExplorerBackend) GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	return ex.store.Do().GetTxProof(unitID, txHash)
}

// GetRoundNumber returns latest round number.
func (ex *ExplorerBackend) GetRoundNumber(ctx context.Context) (uint64, error) {
	return ex.sdk.GetRoundNumber(ctx)
}

func (ex *ExplorerBackend) GetTxHistoryRecords(dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
	return ex.store.Do().GetTxHistoryRecords(dbStartKey, count)
}

func (ex *ExplorerBackend) GetTxHistoryRecordsByKey(hash sdk.PubKeyHash, dbStartKey []byte, count int) ([]*sdk.TxHistoryRecord, []byte, error) {
	return ex.store.Do().GetTxHistoryRecordsByKey(hash, dbStartKey, count)
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
