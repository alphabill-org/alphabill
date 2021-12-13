package abclient

import (
	rpcab "ab-faucet-api/internal/rpc"
	"google.golang.org/grpc"
)

type AlphaBillClient struct {
	config     *AlphaBillClientConfig
	connection *grpc.ClientConn
	client     rpcab.AlphaBillServiceClient
}

func New(config *AlphaBillClientConfig) (*AlphaBillClient, error) {
	conn, err := grpc.Dial(config.Uri, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	return &AlphaBillClient{
		config:     config,
		connection: conn,
		client:     rpcab.NewAlphaBillServiceClient(conn),
	}, nil
}

func (c *AlphaBillClient) Shutdown() error {
	return c.connection.Close()
}
