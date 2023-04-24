// package twb implements token wallet backend
package twb

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/ainvaltin/httpsrv"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
	"github.com/alphabill-org/alphabill/pkg/wallet/broker"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type Configuration interface {
	Client() ABClient
	Storage() (Storage, error)
	BatchSize() int
	HttpServer(http.Handler) http.Server
	Listener() net.Listener
	Logger() log.Logger
}

type ABClient interface {
	SendTransaction(ctx context.Context, tx *txsystem.Transaction) (*txsystem.TransactionResponse, error)
	GetBlocks(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error)
}

type Storage interface {
	Close() error
	GetBlockNumber() (uint64, error)
	SetBlockNumber(blockNumber uint64) error

	SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator wallet.PubKey) error
	SaveTokenType(data *TokenUnitType, proof *wallet.Proof) error
	GetTokenType(id TokenTypeID) (*TokenUnitType, error)
	QueryTokenType(kind Kind, creator wallet.PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error)

	SaveToken(data *TokenUnit, proof *wallet.Proof) error
	GetToken(id TokenID) (*TokenUnit, error)
	QueryTokens(kind Kind, owner wallet.Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error)

	GetTxProof(unitID wallet.UnitID, txHash wallet.TxHash) (*wallet.Proof, error)
}

/*
Run starts the token wallet backend - syncing blocks to local storage and
launching HTTP server to query it.
Run blocks until ctx is cancelled or some unrecoverable error happens, it
always returns non-nil error.
*/
func Run(ctx context.Context, cfg Configuration) error {
	db, err := cfg.Storage()
	if err != nil {
		return fmt.Errorf("failed to get storage: %w", err)
	}
	defer db.Close()

	txs, err := tokens.New()
	if err != nil {
		return fmt.Errorf("failed to create token tx system: %w", err)
	}
	abc := cfg.Client()
	msgBroker := broker.NewBroker()

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		logger := cfg.Logger()
		bp := &blockProcessor{store: db, txs: txs, notify: msgBroker.Notify, log: logger}
		// we act as if all errors returned by block sync are recoverable ie we
		// just retry in a loop until ctx is cancelled
		for {
			logger.Debug("starting block sync")
			err := runBlockSync(ctx, abc.GetBlocks, db.GetBlockNumber, cfg.BatchSize(), bp.ProcessBlock)
			if err != nil {
				logger.Error("synchronizing blocks returned error: ", err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(rand.Int31n(10)+10) * time.Second):
			}
		}
	})

	g.Go(func() error {
		api := &restAPI{
			db:              db,
			convertTx:       txs.ConvertTx,
			sendTransaction: abc.SendTransaction,
			streamSSE:       msgBroker.StreamSSE,
			logErr:          cfg.Logger().Error,
		}
		return httpsrv.Run(ctx, cfg.HttpServer(api.endpoints()), httpsrv.Listener(cfg.Listener()), httpsrv.ShutdownTimeout(5*time.Second))
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

type cfg struct {
	abc     client.AlphabillClientConfig
	boltDB  string
	apiAddr string
	log     log.Logger
}

/*
NewConfig returns Configuration suitable for using as Run parameter.
  - apiAddr: address on which to expose REST API;
  - abURL: AlphaBill backend from where to sync blocks;
  - boltDB: filename (with full path) of the bolt db to use as storage;
  - logger: logger implementation.
*/
func NewConfig(apiAddr, abURL, boltDB string, logger log.Logger) Configuration {
	return &cfg{
		abc:     client.AlphabillClientConfig{Uri: abURL},
		boltDB:  boltDB,
		apiAddr: apiAddr,
		log:     logger,
	}
}

func (c *cfg) Client() ABClient          { return client.New(c.abc) }
func (c *cfg) Storage() (Storage, error) { return newBoltStore(c.boltDB) }
func (c *cfg) BatchSize() int            { return 100 }
func (c *cfg) Logger() log.Logger        { return c.log }
func (c *cfg) Listener() net.Listener    { return nil } // we do set Addr in HttpServer

func (c *cfg) HttpServer(endpoints http.Handler) http.Server {
	return http.Server{
		Addr:              c.apiAddr,
		Handler:           endpoints,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}
