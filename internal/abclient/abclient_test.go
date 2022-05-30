package abclient

import (
	"crypto/sha256"
	"strconv"
	"sync"
	"testing"

	billtx "gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	testserver "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/server"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
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
		_, _ = abclient.SendTransaction(createRandomTx())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_, _ = abclient.GetBlock(1)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		_, _ = abclient.GetMaxBlockNumber()
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
