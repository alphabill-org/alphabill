package rpc

/*
import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

var failingTransactionID = uint256.NewInt(5).Bytes32()

type (
	MockNode struct {
		maxBlockNumber uint64
		transactions   []*types.TransactionOrder
	}
)

func (mn *MockNode) SubmitTx(_ context.Context, tx *types.TransactionOrder) error {
	if bytes.Equal(tx.UnitID(), failingTransactionID[:]) {
		return errors.New("failed")
	}
	if tx != nil {
		mn.transactions = append(mn.transactions, tx)
	}
	return nil
}

func (mn *MockNode) GetBlock(_ context.Context, blockNumber uint64) (*types.Block, error) {
	return &types.Block{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber}}}, nil
}

func (mn *MockNode) GetLatestBlock() (*types.Block, error) {
	return mn.GetBlock(context.Background(), mn.maxBlockNumber)
}

func (mn *MockNode) GetLatestRoundNumber() (uint64, error) {
	return mn.maxBlockNumber, nil
}

func (mn *MockNode) SystemIdentifier() []byte {
	return []byte{0, 1, 0, 0}
}

func TestNewRpcServer_PartitionNodeMissing(t *testing.T) {
	p, err := NewGRPCServer(nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.EqualError(t, err, `partition node which implements the service must be assigned`)
}

func TestNewRpcServer_Ok(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{})
	assert.NotNil(t, p)
	assert.Nil(t, err)
}

func TestRpcServer_GetBlocksOk(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 12})
	require.NoError(t, err)
	res, err := p.GetBlocks(context.Background(), &alphabill.GetBlocksRequest{BlockNumber: 1, BlockCount: 12})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 12)
	require.EqualValues(t, 12, res.MaxBlockNumber)
}

func TestRpcServer_GetBlocksSingleBlock(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 1})
	require.NoError(t, err)
	res, err := p.GetBlocks(context.Background(), &alphabill.GetBlocksRequest{BlockNumber: 1, BlockCount: 1})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 1)
	require.EqualValues(t, 1, res.MaxBlockNumber)
}

func TestRpcServer_FetchNonExistentBlocks_DoesNotPanic(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 7})
	require.NoError(t, err)
	res, err := p.GetBlocks(context.Background(), &alphabill.GetBlocksRequest{BlockNumber: 73, BlockCount: 100})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 0)
	require.EqualValues(t, 7, res.MaxBlockNumber)
}

func TestRpcServer_ProcessTransaction_Fails(t *testing.T) {
	ctx := context.Background()
	con, client := createRpcClient(t, ctx)
	defer con.Close()
	response, err := client.ProcessTransaction(ctx, createTransaction(failingTransactionID))
	assert.Nil(t, response)
	assert.Errorf(t, err, "failed")
}

func TestRpcServer_ProcessTransaction_Ok(t *testing.T) {
	ctx := context.Background()
	con, client := createRpcClient(t, ctx)
	defer con.Close()

	req := createTransaction(uint256.NewInt(1).Bytes32())
	response, err := client.ProcessTransaction(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, response)
}

func createRpcClient(t *testing.T, ctx context.Context) (*grpc.ClientConn, alphabill.AlphabillServiceClient) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	rpcServer, _ := NewGRPCServer(&MockNode{})
	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Error("gRPC server exited with error:", err)
		}
	}()
	t.Cleanup(func() { grpcServer.Stop() })

	d := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(ctx, "", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(d))
	if err != nil {
		t.Fatal(err)
	}
	return conn, alphabill.NewAlphabillServiceClient(conn)
}

func createTransaction(id [32]byte) *types.TransactionOrder {
	bt := &money.TransferAttributes{
		NewBearer:   script.PredicateAlwaysTrue(),
		TargetValue: 1,
		Backlink:    nil,
	}

	txBytes, err := cbor.Marshal(bt)
	if err != nil {
		panic(err)
	}
	return &types.TransactionOrder{
		Payload: &types.Payload{
			UnitID:         id[:],
			Type:           "transfer",
			Attributes:     txBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 0},
		},
		OwnerProof: []byte{1},
	}
}*/
