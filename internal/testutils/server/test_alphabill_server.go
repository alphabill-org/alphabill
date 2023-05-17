package testserver

/*
import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TestAlphabillServiceServer struct {
	// mu mutex guarding all TestAlphabillServiceServer fields
	mu             sync.Mutex
	pubKey         []byte
	maxBlockHeight uint64
	processedTxs   []*types.TransactionOrder
	blocks         map[uint64]func() *types.Block
	//alphabill.UnimplementedAlphabillServiceServer
}

func NewTestAlphabillServiceServer() *TestAlphabillServiceServer {
	return &TestAlphabillServiceServer{blocks: make(map[uint64]func() *types.Block, 100)}
}

func (s *TestAlphabillServiceServer) ProcessTransaction(_ context.Context, tx *types.TransactionOrder) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.processedTxs = append(s.processedTxs, tx)
	return &emptypb.Empty{}, nil
}

func (s *TestAlphabillServiceServer) GetBlock(_ context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blockFunc, ok := s.blocks[req.BlockNo]
	if !ok {
		return nil, fmt.Errorf("block with number %d not found", req.BlockNo)
	}
	return &alphabill.GetBlockResponse{Block: blockFunc()}, nil
}

func (s *TestAlphabillServiceServer) GetBlocks(_ context.Context, req *alphabill.GetBlocksRequest) (*alphabill.GetBlocksResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blocks := make([]*types.Block, 0, req.BlockCount)
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

func (s *TestAlphabillServiceServer) GetRoundNumber(context.Context, *emptypb.Empty) (*alphabill.GetRoundNumberResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &alphabill.GetRoundNumberResponse{RoundNumber: s.maxBlockHeight}, nil
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

func (s *TestAlphabillServiceServer) GetProcessedTransactions() []*types.TransactionOrder {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.processedTxs
}

func (s *TestAlphabillServiceServer) GetAllBlocks() map[uint64]func() *types.Block {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blocks
}

func (s *TestAlphabillServiceServer) SetBlock(blockNo uint64, b *types.Block) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[blockNo] = func() *types.Block {
		return b
	}
}

func (s *TestAlphabillServiceServer) SetBlockFunc(blockNo uint64, blockFunc func() *types.Block) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blocks[blockNo] = blockFunc
}

func StartServer(alphabillService *TestAlphabillServiceServer) (*grpc.Server, net.Addr) {
	lis, err := net.Listen("tcp", "localhost:0")
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
	return grpcServer, lis.Addr()
}

func closeListener(lis net.Listener) {
	_ = lis.Close()
}*/
