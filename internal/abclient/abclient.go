package abclient

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/pkg/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
	"io"
)

type ABClient interface {
	SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error)
	Shutdown()
	IsShutdown() bool
	InitBlockReceiver(blockHeight uint64, terminateAtMaxHeight bool, ch chan<- *alphabill.GetBlocksResponse) error
}

type AlphaBillClientConfig struct {
	Uri string
}

type AlphaBillClient struct {
	config     AlphaBillClientConfig
	connection *grpc.ClientConn
	client     alphabill.AlphaBillServiceClient
}

func New(config AlphaBillClientConfig) (*AlphaBillClient, error) {
	conn, err := grpc.Dial(config.Uri, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return &AlphaBillClient{
		config:     config,
		connection: conn,
		client:     alphabill.NewAlphaBillServiceClient(conn),
	}, nil
}

func (c *AlphaBillClient) InitBlockReceiver(blockHeight uint64, terminateAtMaxHeight bool, ch chan<- *alphabill.GetBlocksResponse) error {
	stream, err := c.client.GetBlocks(context.Background(),
		&alphabill.GetBlocksRequest{
			BlockHeight: blockHeight,
		})
	if err != nil {
		return err
	}

	for {
		getBlocksResponse, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				log.Info("block receiver EOF")
				return nil
			}
			code, ok := status.FromError(err)
			if ok && code.Code() == codes.Canceled {
				log.Info("block receiver cancelled")
				return nil
			}
			return err
		}
		ch <- getBlocksResponse

		if terminateAtMaxHeight && getBlocksResponse.Block.BlockNo == getBlocksResponse.MaxBlockHeight {
			log.Info("block receiver maxBlockHeight reached")
			return nil
		}
	}
}

func (c *AlphaBillClient) SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	return c.client.ProcessTransaction(context.Background(), tx)
}

func (c *AlphaBillClient) Shutdown() {
	if c.IsShutdown() {
		return
	}
	log.Info("shutting down alphabill client")
	err := c.connection.Close()
	if err != nil {
		log.Error("error shutting down alphabill client: ", err)
	}
}

func (c *AlphaBillClient) IsShutdown() bool {
	return c.connection.GetState() == connectivity.Shutdown
}
