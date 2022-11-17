package testserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"google.golang.org/grpc"
)

type TestAlphabillServiceServer struct {
	// mu mutex guarding all TestAlphabillServiceServer fields
	mu             sync.Mutex
	pubKey         []byte
	maxBlockHeight uint64
	processedTxs   []*txsystem.Transaction
	blocks         map[uint64]func() *block.Block
	alphabill.UnimplementedAlphabillServiceServer
}

func NewTestAlphabillServiceServer() *TestAlphabillServiceServer {
	return &TestAlphabillServiceServer{blocks: make(map[uint64]func() *block.Block, 100)}
}

func (s *TestAlphabillServiceServer) ProcessTransaction(_ context.Context, tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processedTxs = append(s.processedTxs, tx)
	return &txsystem.TransactionResponse{Ok: true}, nil
}

func (s *TestAlphabillServiceServer) GetBlock(_ context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blockFunc, f := s.blocks[req.BlockNo]
	if !f {
		return &alphabill.GetBlockResponse{Block: nil, ErrorMessage: fmt.Sprintf("block with number %v not found", req.BlockNo)}, nil
	}
	return &alphabill.GetBlockResponse{Block: blockFunc()}, nil
}

func (s *TestAlphabillServiceServer) GetBlocks(_ context.Context, req *alphabill.GetBlocksRequest) (*alphabill.GetBlocksResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blocks := make([]*block.Block, 0, req.BlockCount)
	maxBlockNumber := util.Min(s.maxBlockHeight, req.BlockNumber+req.BlockCount-1)
	for i := req.BlockNumber; i <= maxBlockNumber; i++ {
		blockFunc, f := s.blocks[i]
		if !f {
			return nil, fmt.Errorf("block with number %v not found", i)
		}
		blocks = append(blocks, blockFunc())
	}
	return &alphabill.GetBlocksResponse{Blocks: blocks, MaxBlockNumber: s.maxBlockHeight}, nil
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

func (s *TestAlphabillServiceServer) SetMaxBlockNumber(maxBlockHeight uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxBlockHeight = maxBlockHeight
}

func (s *TestAlphabillServiceServer) GetProcessedTransactions() []*txsystem.Transaction {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processedTxs
}

func (s *TestAlphabillServiceServer) GetAllBlocks() map[uint64]func() *block.Block {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocks
}

func (s *TestAlphabillServiceServer) SetBlock(blockNo uint64, b *block.Block) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[blockNo] = func() *block.Block {
		return b
	}
}

func (s *TestAlphabillServiceServer) SetBlockFunc(blockNo uint64, blockFunc func() *block.Block) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[blockNo] = blockFunc
}

func StartServer(port int, alphabillService *TestAlphabillServiceServer) *grpc.Server {
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	alphabill.RegisterAlphabillServiceServer(grpcServer, alphabillService)
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
