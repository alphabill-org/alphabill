package wallet

import (
	"context"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"log"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
)

const port = 9111

func TestSync(t *testing.T) {
	// setup wallet
	_ = testutil.DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(Config{
		DbPath:                os.TempDir(),
		Db:                    nil,
		AlphaBillClientConfig: AlphaBillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	k, err := w.db.GetAccountKey()
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	blocks := []*alphabill.GetBlocksResponse{
		{
			MaxBlockHeight: 10,
			Block: &alphabill.Block{
				BlockNo:       1,
				PrevBlockHash: hash.Sum256([]byte{}),
				Transactions: []*transaction.Transaction{
					// random dust transfer can be processed
					{
						UnitId:                hash.Sum256([]byte{0x00}),
						TransactionAttributes: createRandomDustTransferTx(),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentEmpty(),
					},
					// receive transfer of 100 bills
					{
						UnitId:                hash.Sum256([]byte{0x01}),
						TransactionAttributes: createBillTransferTx(k.PubKeyHashSha256),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
					},
					// receive split of 100 bills
					{
						UnitId:                hash.Sum256([]byte{0x02}),
						TransactionAttributes: createBillSplitTx(k.PubKeyHashSha256, 100, 100),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
					},
					// receive swap of 100 bills
					{
						UnitId:                hash.Sum256([]byte{0x03}),
						TransactionAttributes: createRandomSwapTransferTx(k.PubKeyHashSha256),
						Timeout:               1000,
						OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
					},
				},
				UnicityCertificate: []byte{},
			},
		},
	}
	serviceServer := &testAlphaBillServiceServer{ch: make(chan *alphabill.GetBlocksResponse, 100)}
	for _, block := range blocks {
		serviceServer.ch <- block
	}
	serviceServer.CloseChannel()
	server := startServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// verify starting block height
	height, err := w.db.GetBlockHeight()
	require.EqualValues(t, 0, height)
	require.NoError(t, err)

	// verify starting balance
	balance, err := w.GetBalance()
	require.EqualValues(t, 0, balance)
	require.NoError(t, err)

	// when wallet is synced with the node
	err = w.Sync()
	require.NoError(t, err)

	// then block height is increased
	height, err = w.db.GetBlockHeight()
	require.EqualValues(t, 1, height)
	require.NoError(t, err)

	// and balance is increased
	balance, err = w.GetBalance()
	require.EqualValues(t, 300, balance)
	require.NoError(t, err)
}

func TestSyncToMaxBlockHeight(t *testing.T) {
	// setup wallet
	_ = testutil.DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(Config{
		DbPath:                os.TempDir(),
		AlphaBillClientConfig: AlphaBillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	serviceServer := &testAlphaBillServiceServer{ch: make(chan *alphabill.GetBlocksResponse, 100)}
	maxBlockHeight := uint64(3)
	for blockNo := uint64(1); blockNo <= 10; blockNo++ {
		block := &alphabill.GetBlocksResponse{
			MaxBlockHeight: maxBlockHeight,
			Block: &alphabill.Block{
				BlockNo:            blockNo,
				PrevBlockHash:      hash.Sum256([]byte{}),
				Transactions:       []*transaction.Transaction{},
				UnicityCertificate: []byte{},
			},
		}
		serviceServer.ch <- block
	}
	serviceServer.CloseChannel()
	server := startServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// verify starting block height
	height, err := w.db.GetBlockHeight()
	require.EqualValues(t, 0, height)
	require.NoError(t, err)

	// when wallet is synced to max block height
	err = w.SyncToMaxBlockHeight()
	require.NoError(t, err)

	// then block height is exactly equal to max block height, and further blocks are not processed
	height, err = w.db.GetBlockHeight()
	require.EqualValues(t, maxBlockHeight, height)
	require.NoError(t, err)
}

func TestSendInUnsyncedWallet(t *testing.T) {
	// setup wallet
	_ = testutil.DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(Config{
		DbPath:                os.TempDir(),
		AlphaBillClientConfig: AlphaBillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	addBill(t, w, 100)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	// start server
	serviceServer := &testAlphaBillServiceServer{}
	server := startServer(port, serviceServer)
	t.Cleanup(server.GracefulStop)

	// verify wallet has not been synced
	require.Nil(t, w.alphaBillClient)

	// when Send is called in unsynced wallet
	err = w.Send(make([]byte, 33), 50)

	// then transaction is sent
	require.NoError(t, err)
	require.Len(t, serviceServer.processedTxs, 1)

	// and alphabill client has been set
	require.NotNil(t, w.alphaBillClient)
}

func TestCollectDustTimeoutReached(t *testing.T) {
	// setup wallet
	_ = testutil.DeleteWalletDb(os.TempDir())
	w, err := CreateNewWallet(Config{
		DbPath:                os.TempDir(),
		AlphaBillClientConfig: AlphaBillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)
	addBill(t, w, 100)
	addBill(t, w, 200)

	// start server
	serverService := &testAlphaBillServiceServer{ch: make(chan *alphabill.GetBlocksResponse, 100)}
	defer serverService.CloseChannel()
	server := startServer(port, serverService)
	t.Cleanup(server.GracefulStop)

	// when CollectDust is called
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := w.CollectDust()
		wg.Done()
		require.NoError(t, err)
	}()
	waitForExpectedSwap(w)

	// then dc transactions are sent
	require.Len(t, serverService.processedTxs, 2)
	require.NoError(t, err)

	// and dc wg metadata is saved
	require.Len(t, w.dcWg.swaps, 1)
	dcNonce := calculateExpectedDcNonce(t, w)
	require.EqualValues(t, w.dcWg.swaps[*uint256.NewInt(0).SetBytes(dcNonce)], dcTimeoutBlockCount)

	// when dc timeout is reached without receiving dc transfers
	for blockNo := uint64(1); blockNo <= dcTimeoutBlockCount; blockNo++ {
		block := &alphabill.GetBlocksResponse{
			MaxBlockHeight: 100,
			Block: &alphabill.Block{
				BlockNo:            blockNo,
				PrevBlockHash:      hash.Sum256([]byte{}),
				Transactions:       []*transaction.Transaction{},
				UnicityCertificate: []byte{},
			},
		}
		serverService.ch <- block
	}

	// wait for collect dust to finish
	// database will be closed after this point
	wg.Wait()

	// then dc wg is cleared
	require.Len(t, w.dcWg.swaps, 0)
}

type testAlphaBillServiceServer struct {
	pubKey         []byte
	maxBlockHeight uint64
	processedTxs   []*transaction.Transaction
	ch             chan *alphabill.GetBlocksResponse
	alphabill.UnimplementedAlphaBillServiceServer
}

func (s *testAlphaBillServiceServer) GetBlocks(req *alphabill.GetBlocksRequest, stream alphabill.AlphaBillService_GetBlocksServer) error {
	for blockResponse := range s.ch {
		err := stream.Send(blockResponse)
		if err != nil {
			log.Printf("error sending block %s", err)
		}
	}
	return nil
}

func (s *testAlphaBillServiceServer) ProcessTransaction(ctx context.Context, tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	s.processedTxs = append(s.processedTxs, tx)
	return &transaction.TransactionResponse{Ok: true}, nil
}

func (s *testAlphaBillServiceServer) CloseChannel() {
	close(s.ch)
}

func createBillTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&transaction.BillTransfer{
		TargetValue: 100,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		Backlink:    hash.Sum256([]byte{}),
	})
	return tx
}

func createBillSplitTx(pubKeyHash []byte, amount uint64, remainingValue uint64) *anypb.Any {
	tx, _ := anypb.New(&transaction.BillSplit{
		Amount:         amount,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		RemainingValue: remainingValue,
		Backlink:       hash.Sum256([]byte{}),
	})
	return tx
}

func createRandomDcTx() *transaction.Transaction {
	return &transaction.Transaction{
		UnitId:                hash.Sum256([]byte{0x00}),
		TransactionAttributes: createRandomDustTransferTx(),
		Timeout:               1000,
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
}

func createRandomDustTransferTx() *anypb.Any {
	tx, _ := anypb.New(&transaction.TransferDC{
		TargetBearer: script.PredicateAlwaysTrue(),
		Backlink:     hash.Sum256([]byte{}),
		Nonce:        hash.Sum256([]byte{}),
		TargetValue:  100,
	})
	return tx
}

func createRandomSwapTransferTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&transaction.Swap{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		BillIdentifiers: [][]byte{},
		DcTransfers:     []*transaction.Transaction{},
		Proofs:          [][]byte{},
		TargetValue:     100,
	})
	return tx
}

func startServer(port int, alphaBillService *testAlphaBillServiceServer) *grpc.Server {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	alphabill.RegisterAlphaBillServiceServer(grpcServer, alphaBillService)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			defer closeListener(lis)
		}
	}()
	return grpcServer
}

func closeListener(lis net.Listener) {
	_ = lis.Close()
}
