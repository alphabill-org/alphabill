package rpc

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"net"
	"strings"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
)

var failingTransactionID = uint256.NewInt(5)

type (
	MockTransactionProcessor struct{}
	MockLedgerService        struct{}
)

func (mpp *MockTransactionProcessor) Process(gtx transaction.GenericTransaction) error {
	if gtx.UnitID().Eq(failingTransactionID) {
		return errors.New("failed")
	}
	return nil
}

func (mpp *MockTransactionProcessor) Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	return transaction.NewMoneyTx(tx)
}

func (mls *MockLedgerService) GetBlock(request *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	return &alphabill.GetBlockResponse{Block: &alphabill.Block{BlockNo: 1}}, nil
}

func (mls *MockLedgerService) GetMaxBlockNo(request *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	return &alphabill.GetMaxBlockNoResponse{BlockNo: 100}, nil
}

func TestNewRpcServer_ProcessorMissing(t *testing.T) {
	p, err := NewRpcServer(nil, &MockLedgerService{})
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), errstr.NilArgument))
}

func TestNewRpcServer_LedgerServiceMissing(t *testing.T) {
	p, err := NewRpcServer(&MockTransactionProcessor{}, nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), errstr.NilArgument))
}

func TestNewRpcServer_Ok(t *testing.T) {
	p, err := NewRpcServer(&MockTransactionProcessor{}, &MockLedgerService{})
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

	req := createTransaction(uint256.NewInt(1))
	response, err := client.ProcessTransaction(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Ok)
}

func createRpcClient(t *testing.T, ctx context.Context) (*grpc.ClientConn, alphabill.AlphaBillServiceClient) {
	t.Helper()
	processor := &MockTransactionProcessor{}
	LedgerService := &MockLedgerService{}
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	rpcServer, _ := NewRpcServer(processor, LedgerService)
	alphabill.RegisterAlphaBillServiceServer(grpcServer, rpcServer)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Fatal(err)
		}
	}()
	d := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(ctx, "", grpc.WithInsecure(), grpc.WithContextDialer(d))
	if err != nil {
		t.Fatal(err)
	}
	return conn, alphabill.NewAlphaBillServiceClient(conn)
}

func createTransaction(id *uint256.Int) *transaction.Transaction {
	tx := &transaction.Transaction{
		UnitId:                id.Bytes(),
		TransactionAttributes: new(anypb.Any),
		Timeout:               0,
		OwnerProof:            []byte{1},
	}
	bt := &transaction.BillTransfer{
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
