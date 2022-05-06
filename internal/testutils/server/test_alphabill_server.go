package testserver

import (
	"context"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
	"google.golang.org/grpc"
	"log"
	"net"
	"sync"
)

type TestAlphabillServiceServer struct {
	// mu mutex guarding all TestAlphabillServiceServer fields
	mu             sync.Mutex
	pubKey         []byte
	maxBlockHeight uint64
	processedTxs   []*transaction.Transaction
	blocks         map[uint64]*alphabill.GetBlockResponse
	alphabill.UnimplementedAlphabillServiceServer
}

func NewTestAlphabillServiceServer() *TestAlphabillServiceServer {
	return &TestAlphabillServiceServer{blocks: make(map[uint64]*alphabill.GetBlockResponse, 100)}
}

func (s *TestAlphabillServiceServer) ProcessTransaction(_ context.Context, tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processedTxs = append(s.processedTxs, tx)
	return &transaction.TransactionResponse{Ok: true}, nil
}

func (s *TestAlphabillServiceServer) GetBlock(_ context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, f := s.blocks[req.BlockNo]
	if !f {
		return &alphabill.GetBlockResponse{Block: nil, ErrorMessage: fmt.Sprintf("block with number %v not found", req.BlockNo)}, nil
	}
	return res, nil
}

func (s *TestAlphabillServiceServer) GetMaxBlockNo(context.Context, *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &alphabill.GetMaxBlockNoResponse{BlockNo: s.maxBlockHeight}, nil
}

func (s *TestAlphabillServiceServer) GetPubKey() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pubKey
}

func (s *TestAlphabillServiceServer) GetMaxBlockHeight() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.maxBlockHeight
}

func (s *TestAlphabillServiceServer) SetMaxBlockHeight(maxBlockHeight uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxBlockHeight = maxBlockHeight
}

func (s *TestAlphabillServiceServer) GetProcessedTransactions() []*transaction.Transaction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processedTxs
}

func (s *TestAlphabillServiceServer) GetBlocks() map[uint64]*alphabill.GetBlockResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocks
}

func (s *TestAlphabillServiceServer) SetBlock(blockNo uint64, block *alphabill.GetBlockResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[blockNo] = block
}

func StartServer(port int, alphaBillService *TestAlphabillServiceServer) *grpc.Server {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	alphabill.RegisterAlphabillServiceServer(grpcServer, alphaBillService)
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
