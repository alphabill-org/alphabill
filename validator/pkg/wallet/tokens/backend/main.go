package backend

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/ainvaltin/httpsrv"
	tokens2 "github.com/alphabill-org/alphabill/txsystem/tokens"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/debug"
	"github.com/alphabill-org/alphabill/validator/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/validator/pkg/client"
	"github.com/alphabill-org/alphabill/validator/pkg/logger"
	sdk "github.com/alphabill-org/alphabill/validator/pkg/wallet"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/blocksync"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/broker"
)

type Configuration interface {
	Client() (ABClient, error)
	Storage() (Storage, error)
	BatchSize() int
	HttpServer(http.Handler) http.Server
	Listener() net.Listener
	Logger() *slog.Logger
	SystemID() []byte
	APIAddr() string
}

type ABClient interface {
	SendTransaction(ctx context.Context, tx *types.TransactionOrder) error
	GetBlocks(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error)
	GetRoundNumber(ctx context.Context) (uint64, error)
}

type Storage interface {
	Close() error
	GetBlockNumber() (uint64, error)
	SetBlockNumber(blockNumber uint64) error

	SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator sdk.PubKey) error
	SaveTokenType(data *TokenUnitType, proof *sdk.Proof) error
	GetTokenType(id TokenTypeID) (*TokenUnitType, error)
	QueryTokenType(kind Kind, creator sdk.PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error)

	SaveToken(data *TokenUnit, proof *sdk.Proof) error
	RemoveToken(id TokenID) error
	GetToken(id TokenID) (*TokenUnit, error)
	QueryTokens(kind Kind, owner sdk.Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error)

	GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)

	GetFeeCreditBill(unitID types.UnitID) (*FeeCreditBill, error)
	SetFeeCreditBill(fcb *FeeCreditBill, proof *sdk.Proof) error

	GetClosedFeeCredit(fcbID types.UnitID) (*types.TransactionRecord, error)
	SetClosedFeeCredit(fcbID types.UnitID, txr *types.TransactionRecord) error
}

/*
Run starts the tokens backend - syncing blocks to local storage and
launching HTTP server to query it.
Run blocks until ctx is cancelled or some unrecoverable error happens, it
always returns non-nil error.
*/
func Run(ctx context.Context, cfg Configuration) error {
	cfg.Logger().Info(fmt.Sprintf("starting tokens backend: BuildInfo=%s", debug.ReadBuildInfo()))
	db, err := cfg.Storage()
	if err != nil {
		return fmt.Errorf("failed to get storage: %w", err)
	}
	defer db.Close()

	txs, err := tokens2.NewTxSystem(
		cfg.Logger(),
		tokens2.WithTrustBase(map[string]crypto.Verifier{"test": nil}),
		tokens2.WithSystemIdentifier(cfg.SystemID()),
	)
	if err != nil {
		return fmt.Errorf("failed to create token tx system: %w", err)
	}

	g, ctx := errgroup.WithContext(ctx)
	msgBroker := broker.NewBroker(ctx.Done())
	abc, err := cfg.Client()
	if err != nil {
		return fmt.Errorf("failed to create Alphabill client: %w", err)
	}

	g.Go(func() error {
		log := cfg.Logger()
		bp := &blockProcessor{store: db, txs: txs, notify: msgBroker.Notify, log: log}
		// we act as if all errors returned by block sync are recoverable ie we
		// just retry in a loop until ctx is cancelled
		for {
			log.Debug("starting block sync")
			err := runBlockSync(ctx, abc.GetBlocks, db.GetBlockNumber, cfg.BatchSize(), bp.ProcessBlock)
			if err != nil {
				log.Error("synchronizing blocks returned error", logger.Error(err))
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(rand.Int31n(10)+10) * time.Second):
			}
		}
	})

	g.Go(func() error {
		cfg.Logger().Info(fmt.Sprintf("tokens backend REST server starting on %s", cfg.APIAddr()))
		api := &tokensRestAPI{
			db:        db,
			ab:        abc,
			streamSSE: msgBroker.StreamSSE,
			rw:        sdk.ResponseWriter{LogErr: func(err error) { cfg.Logger().Error("REST API error", logger.Error(err)) }},
			systemID:  cfg.SystemID(),
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
	abc      client.AlphabillClientConfig
	boltDB   string
	apiAddr  string
	log      *slog.Logger
	systemID []byte
}

/*
NewConfig returns Configuration suitable for using as Run parameter.
  - apiAddr: address on which to expose REST API;
  - abURL: AlphaBill backend from where to sync blocks;
  - boltDB: filename (with full path) of the bolt db to use as storage;
  - logger: logger implementation.
*/
func NewConfig(apiAddr, abURL, boltDB string, logger *slog.Logger, systemID []byte) Configuration {
	return &cfg{
		abc:      client.AlphabillClientConfig{Uri: abURL},
		boltDB:   boltDB,
		apiAddr:  apiAddr,
		log:      logger,
		systemID: systemID,
	}
}

func (c *cfg) Client() (ABClient, error) { return client.New(c.abc, c.log) }
func (c *cfg) Storage() (Storage, error) { return newBoltStore(c.boltDB) }
func (c *cfg) BatchSize() int            { return 100 }
func (c *cfg) Logger() *slog.Logger      { return c.log }
func (c *cfg) Listener() net.Listener    { return nil } // we do set Addr in HttpServer
func (c *cfg) SystemID() []byte          { return c.systemID }
func (c *cfg) APIAddr() string           { return c.apiAddr }

func (c *cfg) HttpServer(endpoints http.Handler) http.Server {
	return http.Server{
		Addr:              c.apiAddr,
		Handler:           endpoints,
		IdleTimeout:       30 * time.Second,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		// can't set global write timeout here - it'll kill the streaming responses prematurely
		//WriteTimeout:    5 * time.Second,
	}
}