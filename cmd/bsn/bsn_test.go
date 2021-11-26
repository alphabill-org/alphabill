package main

import (
	"context"
	"os"
	"sync"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/payment"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"github.com/stretchr/testify/assert"
)

func TestRunBsn_Ok(t *testing.T) {

	// In case of docker networks, the host might not be localhost.
	host := os.Getenv("NETWORK_HOST")
	if host == "" {
		host = "localhost"
	}
	address := host + ":9543"

	_ = os.Setenv("AB_BSN_SERVER_ADDRESS", address)
	_ = os.Setenv("AB_BSN_INITIAL_BILL_VALUE", "100")

	appStoppedWg := sync.WaitGroup{}
	ctx, _ := async.WithWaitGroup(context.Background())
	ctx, ctxCancel := context.WithCancel(ctx)

	runBSNApp(t, ctx, &appStoppedWg, address)

	// Create the gRPC client
	conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure())
	require.NoError(t, err)
	defer conn.Close()
	paymentsClient := payment.NewPaymentsClient(conn)

	// Test cases
	makeSuccessfulPayment(t, ctx, paymentsClient)
	makeFailingPayment(t, ctx, paymentsClient)

	// Close the app
	ctxCancel()
	// Wait for test asserts to be completed
	appStoppedWg.Wait()
}

func makeSuccessfulPayment(t *testing.T, ctx context.Context, paymentsClient payment.PaymentsClient) {
	paymentResponse, err := paymentsClient.MakePayment(ctx, &payment.PaymentRequest{
		BillId:            1,
		PaymentType:       payment.PaymentRequest_TRANSFER,
		Amount:            0,
		PayeePredicate:    script.PredicateAlwaysTrue(),
		Backlink:          []byte{},
		PredicateArgument: []byte{script.StartByte},
	})
	require.NoError(t, err)
	require.NotEmpty(t, paymentResponse.PaymentId, "Successful payment should return some ID")
}

func makeFailingPayment(t *testing.T, ctx context.Context, paymentsClient payment.PaymentsClient) {
	paymentResponse, err := paymentsClient.MakePayment(ctx, &payment.PaymentRequest{
		BillId:            100,
		PaymentType:       payment.PaymentRequest_TRANSFER,
		Amount:            0,
		PayeePredicate:    script.PredicateAlwaysTrue(),
		Backlink:          []byte{},
		PredicateArgument: []byte{script.StartByte},
	})
	require.Error(t, err)
	require.Nil(t, paymentResponse, "Failing payment should return response")
}

func runBSNApp(t *testing.T, ctx context.Context, appStoppedWg *sync.WaitGroup, address string) {
	config := &configuration{}
	appStoppedWg.Add(1)
	go func() {
		// This returns when the application is stopped.
		err := runBillShardNode(ctx, config)
		assert.NoError(t, err)
		// Sanity check
		assert.EqualValues(t, 100, config.InitialBillValue)
		assert.EqualValues(t, address, config.Server.Address)
		appStoppedWg.Done()
	}()
}
