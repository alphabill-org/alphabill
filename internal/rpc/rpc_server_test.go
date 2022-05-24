package rpc

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var failingTransactionID = uint256.NewInt(5).Bytes32()

type (
	MockNode struct{}
)

func (mn *MockNode) SubmitTx(tx *txsystem.Transaction) error {
	if bytes.Equal(tx.UnitId, failingTransactionID[:]) {
		return errors.New("failed")
	}
	return nil
}

func (mn *MockNode) GetBlock(_ uint64) (*block.Block, error) {
	return &block.Block{BlockNumber: 1}, nil
}

func (mn *MockNode) GetLatestBlock() *block.Block {
	return &block.Block{BlockNumber: 1}
}

func TestNewRpcServer_PartitionNodeMissing(t *testing.T) {
	p, err := NewRpcServer(nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), errstr.NilArgument))
}

func TestNewRpcServer_Ok(t *testing.T) {
	p, err := NewRpcServer(&MockNode{})
	assert.NotNil(t, p)
	assert.Nil(t, err)
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
	rpcServer, _ := NewRpcServer(&MockNode{})
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
