package rpc

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/errors/errstr"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var failingTransactionID = uint256.NewInt(5).Bytes32()

type (
	MockNode struct {
		maxBlockNumber uint64
	}
)

func (mn *MockNode) SubmitTx(tx *txsystem.Transaction) error {
	if bytes.Equal(tx.UnitId, failingTransactionID[:]) {
		return errors.New("failed")
	}
	return nil
}

func (mn *MockNode) GetBlock(blockNumber uint64) (*block.Block, error) {
	return &block.Block{BlockNumber: blockNumber}, nil
}

func (mn *MockNode) GetLatestBlock() *block.Block {
	return &block.Block{BlockNumber: mn.maxBlockNumber}
}

func TestNewRpcServer_PartitionNodeMissing(t *testing.T) {
	p, err := NewGRPCServer(nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), errstr.NilArgument))
}

func TestNewRpcServer_Ok(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{})
	assert.NotNil(t, p)
	assert.Nil(t, err)
}

func TestRpcServer_GetBlocksOk(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 12})
	res, err := p.GetBlocks(nil, &alphabill.GetBlocksRequest{BlockNumber: 1, BlockCount: 12})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 12)
	require.EqualValues(t, 12, res.MaxBlockNumber)
}

func TestRpcServer_GetBlocksSingleBlock(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 1})
	res, err := p.GetBlocks(nil, &alphabill.GetBlocksRequest{BlockNumber: 1, BlockCount: 1})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 1)
	require.EqualValues(t, 1, res.MaxBlockNumber)
}

func TestRpcServer_FetchNonExistentBlocks_DoesNotPanic(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 7})
	res, err := p.GetBlocks(nil, &alphabill.GetBlocksRequest{BlockNumber: 73, BlockCount: 100})
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
	assert.True(t, response.Ok)
}

func createRpcClient(t *testing.T, ctx context.Context) (*grpc.ClientConn, alphabill.AlphabillServiceClient) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	rpcServer, _ := NewGRPCServer(&MockNode{})
	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Fatal(err)
		}
	}()
	d := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(ctx, "", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(d))
	if err != nil {
		t.Fatal(err)
	}
	return conn, alphabill.NewAlphabillServiceClient(conn)
}

func createTransaction(id [32]byte) *txsystem.Transaction {
	tx := &txsystem.Transaction{
		UnitId:                id[:],
		TransactionAttributes: new(anypb.Any),
		Timeout:               0,
		OwnerProof:            []byte{1},
	}
	bt := &money.TransferOrder{
		NewBearer:   script.PredicateAlwaysTrue(),
		TargetValue: 1,
		Backlink:    nil,
	}

	err := anypb.MarshalFrom(tx.TransactionAttributes, bt, proto.MarshalOptions{})
	if err != nil {
		panic(err)
	}
	return tx
}
