package client

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"
	"strings"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/fxamacker/cbor/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	ErrFailedToBroadcastTx = "failed to broadcast transaction"
	ErrTxBufferFull        = "tx buffer is full"
	ErrTxRetryCanceled     = "user canceled tx retry"
)

// ABClient manages connection to alphabill node and implements RPC methods
type ABClient interface {
	SendTransaction(ctx context.Context, tx *types.TransactionOrder) error
	SendTransactionWithRetry(ctx context.Context, tx *types.TransactionOrder, maxTries int) error
	GetBlock(ctx context.Context, blockNumber uint64) ([]byte, error)
	GetBlocks(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error)
	GetRoundNumber(ctx context.Context) (uint64, error)
	Close() error
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

func (c *AlphabillClient) SendTransaction(ctx context.Context, tx *types.TransactionOrder) error {
	defer trackExecutionTime(time.Now(), "sending transaction")

	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		return err
	}
	protoTx := &alphabill.Transaction{Order: txBytes}

	if err := c.connect(); err != nil {
		return err
	}
	_, err = c.client.ProcessTransaction(ctx, protoTx)
	return err
}

func (c *AlphabillClient) SendTransactionWithRetry(ctx context.Context, tx *types.TransactionOrder, maxTries int) error {
	for try := 0; try < maxTries; try++ {
		err := c.SendTransaction(ctx, tx)
		if err == nil {
			return nil
		}
		// error message can also contain stacktrace when node returns aberror, so we check prefix instead of exact match
		if strings.Contains(err.Error(), ErrTxBufferFull) {
			log.Debug("tx buffer full, waiting 1s to retry...")
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return fmt.Errorf(ErrTxRetryCanceled)
			}
		}
		return fmt.Errorf("failed to send transaction: %w", err)
	}
	return fmt.Errorf(ErrFailedToBroadcastTx)
}

func (c *AlphabillClient) GetBlock(ctx context.Context, blockNumber uint64) ([]byte, error) {
	defer trackExecutionTime(time.Now(), fmt.Sprintf("downloading block %d", blockNumber))

	if err := c.connect(); err != nil {
		return nil, err
	}

	res, err := c.client.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: blockNumber})
	if err != nil {
		return nil, err
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
	return res, nil
}

func (c *AlphabillClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	defer trackExecutionTime(time.Now(), "fetching round number")

	if err := c.connect(); err != nil {
		return 0, err
	}

	res, err := c.client.GetRoundNumber(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, err
	}
	return res.RoundNumber, nil
}

func (c *AlphabillClient) Close() error {
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
