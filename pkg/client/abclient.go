package client

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/fxamacker/cbor/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	ErrFailedToBroadcastTx = "failed to broadcast transaction"
	ErrTxBufferFull        = "tx buffer is full"
	ErrTxRetryCanceled     = "user canceled tx retry"
)

type AlphabillClientConfig struct {
	Uri          string
	WaitForReady bool
}

type AlphabillClient struct {
	config     AlphabillClientConfig
	connection *grpc.ClientConn
	client     alphabill.AlphabillServiceClient
	log        *slog.Logger
}

// New creates instance of AlphabillClient
func New(config AlphabillClientConfig, log *slog.Logger) (*AlphabillClient, error) {
	abClient := &AlphabillClient{config: config, log: log}

	if err := abClient.connect(); err != nil {
		return nil, err
	}

	return abClient, nil
}

func (c *AlphabillClient) SendTransaction(ctx context.Context, tx *types.TransactionOrder) error {
	defer trackExecutionTime(time.Now(), "sending transaction", c.log)

	txBytes, err := cbor.Marshal(tx)
	if err != nil {
		return err
	}
	protoTx := &alphabill.Transaction{Order: txBytes}

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
			c.log.DebugContext(ctx, "tx buffer full, waiting 1s to retry...")
			select {
			case <-time.After(time.Second):
				continue
			case <-ctx.Done():
				return errors.New(ErrTxRetryCanceled)
			}
		}
		return fmt.Errorf("failed to send transaction: %w", err)
	}
	return errors.New(ErrFailedToBroadcastTx)
}

func (c *AlphabillClient) GetBlock(ctx context.Context, blockNumber uint64) ([]byte, error) {
	defer trackExecutionTime(time.Now(), fmt.Sprintf("downloading block %d", blockNumber), c.log)

	res, err := c.client.GetBlock(ctx, &alphabill.GetBlockRequest{BlockNo: blockNumber})
	if err != nil {
		return nil, err
	}
	return res.Block, nil
}

func (c *AlphabillClient) GetBlocks(ctx context.Context, blockNumber uint64, blockCount uint64) (res *alphabill.GetBlocksResponse, err error) {
	c.log.DebugContext(ctx, fmt.Sprintf("fetching blocks blocknumber=%d, blockcount=%d", blockNumber, blockCount))
	defer func(t1 time.Time) {
		if res != nil && len(res.Blocks) > 0 {
			trackExecutionTime(t1, fmt.Sprintf("downloading blocks %d-%d", blockNumber, blockNumber+uint64(len(res.Blocks))-1), c.log)
		} else {
			trackExecutionTime(t1, "downloading blocks empty response", c.log)
		}
	}(time.Now())

	res, err = c.client.GetBlocks(ctx, &alphabill.GetBlocksRequest{BlockNumber: blockNumber, BlockCount: blockCount})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *AlphabillClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	defer trackExecutionTime(time.Now(), "fetching round number", c.log)

	res, err := c.client.GetRoundNumber(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, err
	}
	return res.RoundNumber, nil
}

func (c *AlphabillClient) Close() error {
	if err := c.connection.Close(); err != nil {
		return fmt.Errorf("error shutting down alphabill client: %w", err)
	}
	return nil
}

// connect connects to given alphabill node and keeps connection open forever,
// connect can be called any number of times, it does nothing if connection is already established and not shut down.
// Shutdown can be used to shut down the client and terminate the connection.
func (c *AlphabillClient) connect() error {
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

func trackExecutionTime(start time.Time, name string, log *slog.Logger) {
	log.Debug(fmt.Sprintf("%s took %s", name, time.Since(start)))
}
