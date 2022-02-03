package wallet

import (
	"context"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/alphabill/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/crypto/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill-wallet-sdk/internal/testutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"log"
	"net"
	"os"
	"strconv"
	"testing"
)

func TestSync(t *testing.T) {
	// setup wallet
	_ = testutil.DeleteWalletDb(os.TempDir())
	port := 9543
	w, err := CreateNewWallet(&Config{
		DbPath:                os.TempDir(),
		AlphaBillClientConfig: &AlphaBillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	k, err := w.db.GetAccountKey()
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	blocks := []*alphabill.Block{
		{
			BlockNo:       1,
			PrevBlockHash: hash.Sum256([]byte{}),
			Transactions: []*transaction.Transaction{
				// random dust transfer can be processed
				{
					UnitId:                hash.Sum256([]byte{0x00}),
					TransactionAttributes: createDustTransferTx(),
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
					TransactionAttributes: createSwapTx(k.PubKeyHashSha256),
					Timeout:               1000,
					OwnerProof:            script.PredicateArgumentPayToPublicKeyHashDefault([]byte{}, k.PubKey),
				},
			},
			UnicityCertificate: []byte{},
		},
	}
	server := startServer(port, &testAlphaBillServiceServer{blocks: blocks})
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
	port := 9543
	w, err := CreateNewWallet(&Config{
		DbPath:                os.TempDir(),
		AlphaBillClientConfig: &AlphaBillClientConfig{Uri: "localhost:" + strconv.Itoa(port)}},
	)
	t.Cleanup(func() {
		DeleteWallet(w)
	})
	require.NoError(t, err)

	// start server that sends given blocks to wallet
	maxBlockHeight := uint64(3)
	var blocks []*alphabill.Block
	for blockNo := uint64(1); blockNo <= 10; blockNo++ {
		blocks = append(blocks, &alphabill.Block{
			BlockNo:            blockNo,
			PrevBlockHash:      hash.Sum256([]byte{}),
			Transactions:       []*transaction.Transaction{},
			UnicityCertificate: []byte{},
		})
	}
	server := startServer(port, &testAlphaBillServiceServer{blocks: blocks, maxBlockHeight: maxBlockHeight})
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

type testAlphaBillServiceServer struct {
	pubKey         []byte
	blocks         []*alphabill.Block
	maxBlockHeight uint64
	alphabill.UnimplementedAlphaBillServiceServer
}

func (s *testAlphaBillServiceServer) GetBlocks(req *alphabill.GetBlocksRequest, stream alphabill.AlphaBillService_GetBlocksServer) error {
	for _, block := range s.blocks {
		err := stream.Send(&alphabill.GetBlocksResponse{Block: block, MaxBlockHeight: s.maxBlockHeight})
		if err != nil {
			log.Printf("error sending block %s", err)
		}
	}
	return nil
}

func (s *testAlphaBillServiceServer) ProcessTransaction(ctx context.Context, tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	return &transaction.TransactionResponse{Ok: true}, nil
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

func createDustTransferTx() *anypb.Any {
	tx, _ := anypb.New(&transaction.TransferDC{
		TargetBearer: script.PredicateAlwaysTrue(),
		Backlink:     hash.Sum256([]byte{}),
		Nonce:        hash.Sum256([]byte{}),
		TargetValue:  100,
	})
	return tx
}

func createSwapTx(pubKeyHash []byte) *anypb.Any {
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
