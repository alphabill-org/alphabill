package rpc

import (
	"context"
	"net"
	"strings"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/mocks"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	testbytes "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/bytes"
)

var (
	failingTransaction = &transaction.TransactionOrder{
		TransactionId:         []byte{6, 6, 6},
		TransactionAttributes: new(anypb.Any),
		Timeout:               1000,
		OwnerProof:            []byte{},
	}
	_ = anypb.MarshalFrom(failingTransaction.TransactionAttributes,
		&transaction.BillTransfer{
			NewBearer: []byte{},
			Backlink:  []byte{},
		}, proto.MarshalOptions{})
)

type MockTransactionProcessor struct {
}

func (mpp *MockTransactionProcessor) Process(tx *domain.TransactionOrder) error {
	if tx.TransactionId == uint256.NewInt(0).SetBytes(failingTransaction.TransactionId) {
		return errors.New("failed")
	}
	return nil
}

func TestNewTransactionsServer_ProcessorMissing(t *testing.T) {
	p, err := New(nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), errstr.NilArgument))
}

func TestNewTransactionsServer_Ok(t *testing.T) {
	p, err := New(&MockTransactionProcessor{})
	assert.NotNil(t, p)
	assert.Nil(t, err)
}

func TestTransactionsServer_Converter_Fail(t *testing.T) {
	ctx := context.Background()
	mockConverter := new(mocks.TransactionOrderConverter)
	p, err := New(&MockTransactionProcessor{}, Opts.TransactionOrderConverter(mockConverter))
	assert.NotNil(t, p)
	assert.Nil(t, err)

	mockConverter.On("ConvertPbToDomain", mock.Anything).Return(nil, errors.New("failed to convert"))

	res, err := p.ProcessTransaction(ctx, nil)
	require.Nil(t, res)
	require.Errorf(t, err, "failed to convert")
}

func TestTransactionsServer_ProcessTransaction_Fail(t *testing.T) {
	ctx := context.Background()
	con, client := createTransactionClient(t, ctx)
	defer con.Close()
	response, err := client.ProcessTransaction(ctx, failingTransaction)
	assert.Nil(t, response)
	assert.Errorf(t, err, errTransactionOrderProcessingFailed)
}

func TestTransactionsServer_ProcessTransaction_Ok(t *testing.T) {
	ctx := context.Background()
	con, client := createTransactionClient(t, ctx)
	defer con.Close()

	req := &transaction.TransactionOrder{
		TransactionId:         testbytes.RandomBytes(32),
		TransactionAttributes: new(anypb.Any),
		Timeout:               0,
		OwnerProof:            []byte{},
	}
	err := anypb.MarshalFrom(req.TransactionAttributes,
		&transaction.BillTransfer{
			NewBearer: []byte{},
			Backlink:  []byte{},
		}, proto.MarshalOptions{})
	require.NoError(t, err)

	response, err := client.ProcessTransaction(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.True(t, response.Ok)
}

func createTransactionClient(t *testing.T, ctx context.Context) (*grpc.ClientConn, transaction.TransactionsClient) {
	t.Helper()
	processor := &MockTransactionProcessor{}
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	transactionsServer, _ := New(processor)
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
