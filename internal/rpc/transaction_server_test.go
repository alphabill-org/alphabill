package rpc

import (
	"context"
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

type MockTransactionProcessor struct {
}

func (mpp *MockTransactionProcessor) Process(gtx transaction.GenericTransaction) error {
	if gtx.UnitId().Eq(failingTransactionID) {
		return errors.New("failed")
	}
	return nil
}

func TestNewTransactionServer_ProcessorMissing(t *testing.T) {
	p, err := NewTransactionsServer(nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), errstr.NilArgument))
}

func TestNewTransactionServer_Ok(t *testing.T) {
	p, err := NewTransactionsServer(&MockTransactionProcessor{})
	assert.NotNil(t, p)
	assert.Nil(t, err)
}

func TestTransactionServer_ProcessTransaction_Fails(t *testing.T) {
	ctx := context.Background()
	con, client := createTransactionsClient(t, ctx)
	defer con.Close()
	response, err := client.ProcessTransaction(ctx, createTransaction(failingTransactionID))
	assert.Nil(t, response)
	assert.Errorf(t, err, "failed")
}

func TestTransactionServer_ProcessTransaction_Ok(t *testing.T) {
	ctx := context.Background()
	con, client := createTransactionsClient(t, ctx)
	defer con.Close()

	req := createTransaction(uint256.NewInt(1))
	response, err := client.ProcessTransaction(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Ok)
}

func createTransactionsClient(t *testing.T, ctx context.Context) (*grpc.ClientConn, transaction.TransactionsClient) {
	t.Helper()
	processor := &MockTransactionProcessor{}
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	transactionsServer, _ := NewTransactionsServer(processor)
	transaction.RegisterTransactionsServer(grpcServer, transactionsServer)
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
	return conn, transaction.NewTransactionsClient(conn)
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
