package client

import (
	"context"
	"crypto/sha256"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	testserver "github.com/alphabill-org/alphabill/internal/testutils/server"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	billtx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

const port = 9222

// TestRaceConditions meant to be run with -race flag
func TestRaceConditions(t *testing.T) {
	// start mock ab server
	serviceServer := testserver.NewTestAlphabillServiceServer()
	server := testserver.StartServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// create ab client
	abclient := New(AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)})
	t.Cleanup(func() { _ = abclient.Shutdown() })

	// do async operations on abclient
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		_, _ = abclient.SendTransaction(context.Background(), createRandomTx())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_, _ = abclient.GetBlock(context.Background(), 1)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_, _ = abclient.GetMaxBlockNumber(context.Background())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		abclient.IsShutdown()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_ = abclient.Shutdown()
		wg.Done()
	}()

	wg.Wait()
}

func TestTimeout(t *testing.T) {
	server, client := startServerAndCreateClient(t)

	// set client timeout to a low value, so the request times out
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// make GetBlock request wait for some time
	server.SetBlockFunc(0, func() *block.Block {
		time.Sleep(100 * time.Millisecond)
		return &block.Block{
			BlockNumber:        0,
			PreviousBlockHash:  hash.Sum256([]byte{}),
			Transactions:       []*txsystem.Transaction{},
			UnicityCertificate: &certificates.UnicityCertificate{},
		}
	})

	// fetch the block and verify request is timed out
	b, err := client.GetBlock(ctx, 0)
	require.Nil(t, b)
	require.ErrorContains(t, err, "deadline exceeded")
}

func createRandomTransfer() *anypb.Any {
	tx, _ := anypb.New(&billtx.TransferOrder{
		TargetValue: 100,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(sha256.New().Sum([]byte{0})),
		Backlink:    hash.Sum256([]byte{}),
	})
	return tx
}

func createRandomTx() *txsystem.Transaction {
	return &txsystem.Transaction{
		UnitId:                hash.Sum256([]byte{0x00}),
		TransactionAttributes: createRandomTransfer(),
		Timeout:               1000,
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
}

func startServerAndCreateClient(t *testing.T) (*testserver.TestAlphabillServiceServer, *AlphabillClient) {
	// start mock ab server
	serviceServer := testserver.NewTestAlphabillServiceServer()
	server := testserver.StartServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// create ab client
	abclient := New(AlphabillClientConfig{Uri: "localhost:" + strconv.Itoa(port)})
	t.Cleanup(func() { _ = abclient.Shutdown() })
	return serviceServer, abclient
}
