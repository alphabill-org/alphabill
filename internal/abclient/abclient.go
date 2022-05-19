package abclient

import (
	"context"
	"sync"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// ABClient manages connection to alphabill node and implements RPC methods
type ABClient interface {
	SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error)
	GetBlock(blockNo uint64) (*block.Block, error)
	GetMaxBlockNo() (uint64, error)
	Shutdown()
	IsShutdown() bool
}

type AlphabillClientConfig struct {
	Uri              string
	RequestTimeoutMs uint64
}

type AlphabillClient struct {
	config     AlphabillClientConfig
	connection *grpc.ClientConn
	client     alphabill.AlphabillServiceClient

	// mu mutex guarding mutable fields (connection and client)
	mu sync.RWMutex
}

// New creates instance of AlphabillClient
func New(config AlphabillClientConfig) *AlphabillClient {
	return &AlphabillClient{config: config}
}

func (c *AlphabillClient) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	err := c.connect()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if c.config.RequestTimeoutMs > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(c.config.RequestTimeoutMs)*time.Millisecond)
		defer cancel()
		ctx = ctxTimeout
	}
	return c.client.ProcessTransaction(ctx, tx)
}

func (c *AlphabillClient) GetBlock(blockNo uint64) (*block.Block, error) {
	err := c.connect()
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if c.config.RequestTimeoutMs > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(c.config.RequestTimeoutMs)*time.Millisecond)
		defer cancel()
		ctx = ctxTimeout
	}
	res, err := c.client.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: blockNo})
	if err != nil {
		return nil, err
	}
	if res.ErrorMessage != "" {
		return nil, errors.New(res.ErrorMessage)
	}
	return res.Block, nil
}

func (c *AlphabillClient) GetMaxBlockNo() (uint64, error) {
	err := c.connect()
	if err != nil {
		return 0, err
	}
	ctx := context.Background()
	if c.config.RequestTimeoutMs > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(c.config.RequestTimeoutMs)*time.Millisecond)
		defer cancel()
		ctx = ctxTimeout
	}
	res, err := c.client.GetMaxBlockNo(ctx, &alphabill.GetMaxBlockNoRequest{})
	if err != nil {
		return 0, err
	}
	if res.ErrorMessage != "" {
		return 0, errors.New(res.ErrorMessage)
	}
	return res.BlockNo, nil
}

func (c *AlphabillClient) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.isShutdown() {
		return
	}
	log.Info("shutting down alphabill client")
	err := c.connection.Close()
	if err != nil {
		log.Error("error shutting down alphabill client: ", err)
	}
}

func (c *AlphabillClient) IsShutdown() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isShutdown()
}

func (c *AlphabillClient) isShutdown() bool {
	return c.connection == nil || c.connection.GetState() == connectivity.Shutdown
}

// connect connects to given alphabill node and keeps connection open forever,
// connect can be called any number of times, it does nothing if connection is already established and not shut down.
// Shutdown can be used to shut down the client and terminate the connection.
func (c *AlphabillClient) connect() error {
	if !c.IsShutdown() {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isShutdown() {
		return nil
	}

	conn, err := grpc.Dial(c.config.Uri, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	c.connection = conn
	c.client = alphabill.NewAlphabillServiceClient(conn)
	return nil
}
