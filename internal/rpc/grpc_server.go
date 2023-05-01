package rpc

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
)

var receivedTransactionsGRPCMeter = metrics.GetOrRegisterCounter("transactions/grpc/received")
var receivedInvalidTransactionsGRPCMeter = metrics.GetOrRegisterCounter("transactions/grpc/invalid")

type (
	grpcServer struct {
		alphabill.UnimplementedAlphabillServiceServer
		node                  partitionNode
		maxGetBlocksBatchSize uint64
	}

	partitionNode interface {
		SubmitTx(ctx context.Context, tx *txsystem.Transaction) error
		GetBlock(ctx context.Context, blockNr uint64) (*block.Block, error)
		GetLatestBlock() (*block.Block, error)
		GetLatestRoundNumber() (uint64, error)
		SystemIdentifier() []byte
	}
)

func NewGRPCServer(node partitionNode, opts ...Option) (*grpcServer, error) {
	if node == nil {
		return nil, fmt.Errorf("partition node which implements the service must be assigned")
	}

	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	if options.maxGetBlocksBatchSize < 1 {
		return nil, fmt.Errorf("server-max-get-blocks-batch-size cannot be less than one, got %d", options.maxGetBlocksBatchSize)
	}

	return &grpcServer{
		node:                  node,
		maxGetBlocksBatchSize: options.maxGetBlocksBatchSize,
	}, nil
}

func (r *grpcServer) ProcessTransaction(ctx context.Context, tx *txsystem.Transaction) (*emptypb.Empty, error) {
	receivedTransactionsGRPCMeter.Inc(1)
	if err := r.node.SubmitTx(ctx, tx); err != nil {
		receivedInvalidTransactionsGRPCMeter.Inc(1)
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (r *grpcServer) GetBlock(ctx context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	b, err := r.node.GetBlock(ctx, req.BlockNo)
	if err != nil {
		return nil, err
	}
	return &alphabill.GetBlockResponse{Block: b}, nil
}

func (r *grpcServer) GetRoundNumber(_ context.Context, _ *emptypb.Empty) (*alphabill.GetRoundNumberResponse, error) {
	latestRn, err := r.node.GetLatestRoundNumber()
	if err != nil {
		return nil, err
	}
	return &alphabill.GetRoundNumberResponse{RoundNumber: latestRn}, nil
}

func (r *grpcServer) GetBlocks(ctx context.Context, req *alphabill.GetBlocksRequest) (*alphabill.GetBlocksResponse, error) {
	if err := verifyRequest(req); err != nil {
		return nil, fmt.Errorf("invalid get blocks request: %w", err)
	}
	latestBlock, err := r.node.GetLatestBlock()
	if err != nil {
		return nil, err
	}
	latestRn, err := r.node.GetLatestRoundNumber()
	if err != nil {
		return nil, err
	}

	maxBlockCount := util.Min(req.BlockCount, r.maxGetBlocksBatchSize)
	batchMaxBlockNumber := util.Min(req.BlockNumber+maxBlockCount-1, latestBlock.UnicityCertificate.InputRecord.RoundNumber)
	batchSize := uint64(0)
	if batchMaxBlockNumber >= req.BlockNumber {
		batchSize = batchMaxBlockNumber - req.BlockNumber + 1
	}
	res := make([]*block.Block, 0, batchSize)
	for blockNumber := req.BlockNumber; blockNumber <= batchMaxBlockNumber; blockNumber++ {
		b, err := r.node.GetBlock(ctx, blockNumber)
		if err != nil {
			return nil, err
		}
		if b == nil {
			continue
		}
		res = append(res, b)
	}
	return &alphabill.GetBlocksResponse{Blocks: res, MaxBlockNumber: latestBlock.UnicityCertificate.InputRecord.RoundNumber, MaxRoundNumber: latestRn, BatchMaxBlockNumber: batchMaxBlockNumber}, nil
}

func verifyRequest(req *alphabill.GetBlocksRequest) error {
	if req.BlockNumber < 1 {
		return fmt.Errorf("block number cannot be less than one, got %d", req.BlockNumber)
	}
	if req.BlockCount < 1 {
		return fmt.Errorf("block count cannot be less than one, got %d", req.BlockCount)
	}
	return nil
}
