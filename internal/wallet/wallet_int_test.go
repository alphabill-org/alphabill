package wallet

import (
	"alphabill-wallet-sdk/internal/abclient"
	"alphabill-wallet-sdk/internal/alphabill/script"
	"alphabill-wallet-sdk/internal/crypto/hash"
	"alphabill-wallet-sdk/internal/rpc/alphabill"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"alphabill-wallet-sdk/internal/testutil"
	"alphabill-wallet-sdk/pkg/wallet/config"
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"log"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestWalletCanProcessBlocks(t *testing.T) {
	testutil.DeleteWalletDb()
	w, err := CreateNewWallet()
	defer DeleteWallet(w)
	require.NoError(t, err)

	k, err := w.db.GetKey()
	require.NoError(t, err)

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
	port := 9543
	server := startServer(port, &testAlphaBillServiceServer{blocks: blocks})
	defer server.GracefulStop()

	require.EqualValues(t, 0, w.db.GetBlockHeight())
	require.EqualValues(t, 0, w.GetBalance())
	err = w.Sync(&config.AlphaBillClientConfig{Uri: "localhost:" + strconv.Itoa(port)})
	require.NoError(t, err)

	waitForShutdown(w.alphaBillClient)

	require.EqualValues(t, 1, w.db.GetBlockHeight())
	require.EqualValues(t, 300, w.GetBalance())
}

type testAlphaBillServiceServer struct {
	pubKey []byte
	blocks []*alphabill.Block
	alphabill.UnimplementedAlphaBillServiceServer
}

func (s *testAlphaBillServiceServer) GetBlocks(req *alphabill.GetBlocksRequest, stream alphabill.AlphaBillService_GetBlocksServer) error {
	for _, block := range s.blocks {
		err := stream.Send(block)
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
	tx, _ := anypb.New(&transaction.DustTransfer{
		NewBearer:   script.PredicateAlwaysTrue(),
		Backlink:    hash.Sum256([]byte{}),
		Nonce:       hash.Sum256([]byte{}),
		TargetValue: 100,
	})
	return tx
}

func createSwapTx(pubKeyHash []byte) *anypb.Any {
	tx, _ := anypb.New(&transaction.Swap{
		OwnerCondition:     script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		BillIdentifiers:    [][]byte{},
		DustTransferOrders: []*transaction.DustTransfer{},
		Proofs:             [][]byte{},
		TargetValue:        100,
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
			defer lis.Close()
		}
	}()
	return grpcServer
}

func waitForShutdown(abClient abclient.ABClient) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		if abClient.IsShutdown() || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
