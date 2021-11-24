package rpc

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/payment"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"net"
	"strconv"
	"strings"
	"testing"
)

var paymentProcessingFails = &payment.PaymentRequest{
	BillId:            1000,
	PaymentType:       payment.PaymentRequest_TRANSFER,
	Amount:            0,
	PayeePredicate:    make([]byte, 32),
	Backlink:          make([]byte, 32),
	PredicateArgument: make([]byte, 32),
}

type MockPaymentProcessor struct {
}

func (mpp *MockPaymentProcessor) Process(payment *state.PaymentOrder) (string, error) {
	if payment.BillID == paymentProcessingFails.BillId {
		return "", errors.New("failed")
	}
	return "TEST", nil
}

func (mpp *MockPaymentProcessor) Status(paymentID string) (interface{}, error) {
	if paymentID == strconv.FormatUint(paymentProcessingFails.BillId, 10) {
		return nil, errors.New("failed")
	}
	return "OK", nil
}

func TestNewPaymentsServer_PaymentProcessorMissing(t *testing.T) {
	p, err := New(nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), errstr.NilArgument))
}

func TestNewPaymentsServer_Ok(t *testing.T) {
	p, err := New(&MockPaymentProcessor{})
	assert.NotNil(t, p)
	assert.Nil(t, err)
}

func TestPaymentsServer_MakePayment_ProcessingFails(t *testing.T) {
	ctx := context.Background()
	con, client := createClient(t, ctx)
	defer con.Close()
	response, err := client.MakePayment(ctx, paymentProcessingFails)
	assert.Nil(t, response)
	assert.True(t, strings.Contains(err.Error(), errPaymentOrderProcessingFailed))
}

func TestPaymentsServer_MakePayment_Ok(t *testing.T) {
	ctx := context.Background()
	con, client := createClient(t, ctx)
	defer con.Close()

	req := &payment.PaymentRequest{
		BillId:            1,
		PaymentType:       payment.PaymentRequest_TRANSFER,
		Amount:            0,
		PayeePredicate:    make([]byte, 32),
		Backlink:          make([]byte, 32),
		PredicateArgument: make([]byte, 32),
	}
	response, err := client.MakePayment(ctx, req)
	assert.Nil(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "TEST", response.PaymentId)
}

func TestPaymentsServer_PaymentStatus_ProcessingFails(t *testing.T) {
	ctx := context.Background()
	con, client := createClient(t, ctx)
	defer con.Close()

	response, err := client.PaymentStatus(ctx,
		&payment.PaymentStatusRequest{PaymentId: strconv.FormatUint(paymentProcessingFails.BillId, 10)},
	)
	assert.Nil(t, response)
	assert.True(t, strings.Contains(err.Error(), errPaymentStatusProcessingFailed))
}

func TestPaymentsServer_PaymentStatus_Ok(t *testing.T) {
	ctx := context.Background()
	con, client := createClient(t, ctx)
	defer con.Close()

	response, err := client.PaymentStatus(ctx,
		&payment.PaymentStatusRequest{PaymentId: "10"},
	)
	assert.Nil(t, err)
	assert.True(t, response.Status)
}

func createClient(t *testing.T, ctx context.Context) (*grpc.ClientConn, payment.PaymentsClient) {
	t.Helper()
	processor := &MockPaymentProcessor{}
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	paymentServer, _ := New(processor)
	payment.RegisterPaymentsServer(grpcServer, paymentServer)
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
	return conn, payment.NewPaymentsClient(conn)
}
