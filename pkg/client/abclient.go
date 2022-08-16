package client

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

// ABClient manages connection to alphabill node and implements RPC methods
type ABClient interface {
	SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error)
	GetBlock(blockNumber uint64) (*block.Block, error)
	GetBlocks(ctx context.Context, blockNumberFrom, blockNumberUntil uint64, ch chan<- *block.Block) error
	GetMaxBlockNumber() (uint64, error)
	Shutdown() error
	IsShutdown() bool
}

type AlphabillClientConfig struct {
	Uri              string
	RequestTimeoutMs uint64
	WaitForReady     bool
}

type AlphabillClient struct {
	config     AlphabillClientConfig
	connection *grpc.ClientConn
	client     alphabill.AlphabillServiceClient

	// mu mutex guarding mutable fields (connection and client)
	mu sync.RWMutex

	cancelCh chan bool
}

// New creates instance of AlphabillClient
func New(config AlphabillClientConfig) *AlphabillClient {
	return &AlphabillClient{config: config, cancelCh: make(chan bool, 1)}
}

func (c *AlphabillClient) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	err := c.connect()
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	ctx := context.Background()
	if c.config.RequestTimeoutMs > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(c.config.RequestTimeoutMs)*time.Millisecond)
		defer cancel()
		ctx = ctxTimeout
	}
	return c.client.ProcessTransaction(ctx, tx, grpc.WaitForReady(c.config.WaitForReady))
}

func (c *AlphabillClient) GetBlock(blockNo uint64) (*block.Block, error) {
	err := c.connect()
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	ctx := context.Background()
	if c.config.RequestTimeoutMs > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(c.config.RequestTimeoutMs)*time.Millisecond)
		defer cancel()
		ctx = ctxTimeout
	}
	res, err := c.client.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: blockNo}, grpc.WaitForReady(c.config.WaitForReady))
	if err != nil {
		return nil, err
	}
	if res.ErrorMessage != "" {
		return nil, errors.New(res.ErrorMessage)
	}
	return res.Block, nil
}

func (c *AlphabillClient) GetBlocks(ctx context.Context, from uint64, until uint64, ch chan<- *block.Block) error {
	err := c.connect()
	if err != nil {
		return err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	stream, err := c.client.GetBlocks(ctx, &alphabill.GetBlocksRequest{BlockNumberFrom: from, BlockNumberUntil: until})
	if err != nil {
		return err
	}
	for {
		select {
		case <-c.cancelCh: // canceled from shutdown
			return nil
		case <-ctx.Done(): // canceled by user or error in block receiver
			return nil
		default:
			res, err := stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
			ch <- res.Block
		}
	}
}

func (c *AlphabillClient) GetMaxBlockNumber() (uint64, error) {
	err := c.connect()
	if err != nil {
		return 0, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	ctx := context.Background()
	if c.config.RequestTimeoutMs > 0 {
		ctxTimeout, cancel := context.WithTimeout(ctx, time.Duration(c.config.RequestTimeoutMs)*time.Millisecond)
		defer cancel()
		ctx = ctxTimeout
	}
	res, err := c.client.GetMaxBlockNo(ctx, &alphabill.GetMaxBlockNoRequest{}, grpc.WaitForReady(c.config.WaitForReady))
	if err != nil {
		return 0, err
	}
	if res.ErrorMessage != "" {
		return 0, errors.New(res.ErrorMessage)
	}
	return res.BlockNo, nil
}

func (c *AlphabillClient) Shutdown() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.isShutdown() {
		// signal GetBlocks rpc stream to close
		select {
		case c.cancelCh <- true:
		default:
		}

		err := c.connection.Close()
		if err != nil {
			return errors.Wrap(err, "error shutting down alphabill client")
		}
	}
	return nil
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
