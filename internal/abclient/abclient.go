package abclient

import (
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/pkg/log"
	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type ABClient interface {
	SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error)
	Shutdown()
	IsShutdown() bool
	InitBlockReceiver(blockHeight uint64, ch chan<- *alphabill.Block) error
}

type AlphaBillClient struct {
	config     *AlphaBillClientConfig
	connection *grpc.ClientConn
	client     alphabill.AlphaBillServiceClient
}

func New(config *AlphaBillClientConfig) (*AlphaBillClient, error) {
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

func (c *AlphaBillClient) InitBlockReceiver(blockHeight uint64, ch chan<- *alphabill.Block) error {
	stream, err := c.client.GetBlocks(context.Background(),
		&alphabill.GetBlocksRequest{
			BlockHeight: blockHeight,
		})
	if err != nil {
		return err
	}

	for {
		block, err := stream.Recv()
		if err != nil {
			return err
		}
		ch <- block
	}
}

func (c *AlphaBillClient) SendTransaction(tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	return c.client.ProcessTransaction(context.Background(), tx)
}

func (c *AlphaBillClient) Shutdown() {
	if c.IsShutdown() {
		return
	}
	err := c.connection.Close()
	if err != nil {
		log.Error("error shutting down alphabill client", err)
	}
}

func (c *AlphaBillClient) IsShutdown() bool {
	return c.connection.GetState() == connectivity.Shutdown
}
