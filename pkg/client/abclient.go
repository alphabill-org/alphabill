package client

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ABClient manages connection to alphabill node and implements RPC methods
type ABClient interface {
	SendTransaction(ctx context.Context, tx *txsystem.Transaction) (*txsystem.TransactionResponse, error)
	GetBlock(ctx context.Context, blockNumber uint64) (*block.Block, error)
	GetBlocks(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error)
	GetMaxBlockNumber(ctx context.Context) (uint64, uint64, error) // latest persisted block number, latest round number
	Shutdown() error
}

type AlphabillClientConfig struct {
	Uri          string
	WaitForReady bool
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

func (c *AlphabillClient) SendTransaction(ctx context.Context, tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	defer trackExecutionTime(time.Now(), "sending transaction")

	if err := c.connect(); err != nil {
		return nil, err
	}

	return c.client.ProcessTransaction(ctx, tx)
}

func (c *AlphabillClient) GetBlock(ctx context.Context, blockNumber uint64) (*block.Block, error) {
	defer trackExecutionTime(time.Now(), fmt.Sprintf("downloading block %d", blockNumber))

	if err := c.connect(); err != nil {
		return nil, err
	}

	res, err := c.client.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: blockNumber})
	if err != nil {
		return nil, err
	}
	if res.ErrorMessage != "" {
		return nil, errors.New(res.ErrorMessage)
	}
	return res.Block, nil
}

func (c *AlphabillClient) GetBlocks(ctx context.Context, blockNumber uint64, blockCount uint64) (res *alphabill.GetBlocksResponse, err error) {
	log.Debug("fetching blocks blocknumber=", blockNumber, " blockcount=", blockCount)
	defer func(t1 time.Time) {
		if res != nil && len(res.Blocks) > 0 {
			trackExecutionTime(t1, fmt.Sprintf("downloading blocks %d-%d", blockNumber, blockNumber+uint64(len(res.Blocks))-1))
		} else {
			trackExecutionTime(t1, "downloading blocks empty response")
		}
	}(time.Now())

	if err := c.connect(); err != nil {
		return nil, err
	}

	res, err = c.client.GetBlocks(ctx, &alphabill.GetBlocksRequest{BlockNumber: blockNumber, BlockCount: blockCount})
	if err != nil {
		return nil, err
	}
	if res.ErrorMessage != "" {
		return nil, errors.New(res.ErrorMessage)
	}
	return res, nil
}

func (c *AlphabillClient) GetMaxBlockNumber(ctx context.Context) (uint64, uint64, error) {
	defer trackExecutionTime(time.Now(), "fetching max block number")

	if err := c.connect(); err != nil {
		return 0, 0, err
	}

	res, err := c.client.GetMaxBlockNo(ctx, &alphabill.GetMaxBlockNoRequest{})
	if err != nil {
		return 0, 0, err
	}
	if res.ErrorMessage != "" {
		return 0, 0, errors.New(res.ErrorMessage)
	}
	return res.BlockNo, res.MaxRoundNumber, nil
}

func (c *AlphabillClient) Shutdown() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connection != nil {
		con := c.connection
		c.connection = nil
		c.client = nil
		if err := con.Close(); err != nil {
			return errors.Wrap(err, "error shutting down alphabill client")
		}
	}
	return nil
}

// connect connects to given alphabill node and keeps connection open forever,
// connect can be called any number of times, it does nothing if connection is already established and not shut down.
// Shutdown can be used to shut down the client and terminate the connection.
func (c *AlphabillClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connection != nil {
		return nil
	}

	callOpts := []grpc.CallOption{grpc.MaxCallSendMsgSize(1024 * 1024 * 4), grpc.MaxCallRecvMsgSize(math.MaxInt32)}
	if c.config.WaitForReady {
		callOpts = append(callOpts, grpc.WaitForReady(c.config.WaitForReady))
	}
	conn, err := grpc.Dial(
		c.config.Uri,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(callOpts...))
	if err != nil {
		return fmt.Errorf("failed to dial gRPC connection: %w", err)
	}
	c.connection = conn
	c.client = alphabill.NewAlphabillServiceClient(conn)
	return nil
}

func trackExecutionTime(start time.Time, name string) {
	log.Debug(name, " took ", time.Since(start))
}
