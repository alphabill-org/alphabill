package abclient

import (
	"crypto/sha256"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	billtx "gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	testserver "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/server"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"google.golang.org/protobuf/types/known/anypb"
	"strconv"
	"sync"
	"testing"
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
	t.Cleanup(abclient.Shutdown)

	// do async operations on abclient
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		abclient.SendTransaction(createRandomTx())
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		abclient.GetBlock(1)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		abclient.GetMaxBlockNo()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		abclient.IsShutdown()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		abclient.Shutdown()
		wg.Done()
	}()

	wg.Wait()
}

func createRandomTransfer() *anypb.Any {
	tx, _ := anypb.New(&billtx.BillTransfer{
		TargetValue: 100,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(sha256.New().Sum([]byte{0})),
		Backlink:    hash.Sum256([]byte{}),
	})
	return tx
}

func createRandomTx() *transaction.Transaction {
	return &transaction.Transaction{
		UnitId:                hash.Sum256([]byte{0x00}),
		TransactionAttributes: createRandomTransfer(),
		Timeout:               1000,
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
}
