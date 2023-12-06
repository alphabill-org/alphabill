package testserver

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TestAlphabillServiceServer struct {
	// mu mutex guarding all TestAlphabillServiceServer fields
	mu             sync.Mutex
	pubKey         []byte
	maxBlockHeight uint64
	processedTxs   []*types.TransactionOrder
	processTxError error
	blocks         map[uint64]func() *types.Block
	alphabill.UnimplementedAlphabillServiceServer
}

func NewTestAlphabillServiceServer() *TestAlphabillServiceServer {
	return &TestAlphabillServiceServer{blocks: make(map[uint64]func() *types.Block, 100)}
}

func (s *TestAlphabillServiceServer) ProcessTransaction(_ context.Context, tx *alphabill.Transaction) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	txo := &types.TransactionOrder{}
	if err := cbor.Unmarshal(tx.Order, txo); err != nil {
		return &emptypb.Empty{}, err
	}
	s.processedTxs = append(s.processedTxs, txo)
	return &emptypb.Empty{}, s.processTxError
}

func (s *TestAlphabillServiceServer) GetBlock(_ context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blockFunc, ok := s.blocks[req.BlockNo]
	if !ok {
		return nil, fmt.Errorf("block with number %d not found", req.BlockNo)
	}
	block := blockFunc()
	blockBytes, err := toBytes(block)
	if err != nil {
		return nil, err
	}
	return &alphabill.GetBlockResponse{Block: blockBytes}, nil
}

func toBytes(block *types.Block) ([]byte, error) {
	blockBytes, err := cbor.Marshal(block)
	if err != nil {
		return nil, err
	}
	return blockBytes, nil
}

func (s *TestAlphabillServiceServer) GetBlocks(_ context.Context, req *alphabill.GetBlocksRequest) (*alphabill.GetBlocksResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blocks := make([][]byte, 0, req.BlockCount)
	maxBlockNumber := min(s.maxBlockHeight, req.BlockNumber+req.BlockCount-1)
	for i := req.BlockNumber; i <= maxBlockNumber; i++ {
		blockFunc, f := s.blocks[i]
		if !f {
			return nil, fmt.Errorf("block with number %v not found", i)
		}
		bytes, err := toBytes(blockFunc())
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, bytes)
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

func (s *TestAlphabillServiceServer) SetProcessTxError(processTxError error) {
	s.processTxError = processTxError
}

func StartServer(alphabillService *TestAlphabillServiceServer, tp trace.TracerProvider) (*grpc.Server, net.Addr) {
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tp))))
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
}
