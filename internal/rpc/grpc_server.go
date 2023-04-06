package rpc

import (
	"context"

	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/errors/errstr"
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
		SubmitTx(tx *txsystem.Transaction) error
		GetBlock(blockNr uint64) (*block.Block, error)
		GetLatestBlock() (*block.Block, error)
		GetLatestRoundNumber() (uint64, error)
		SystemIdentifier() []byte
	}
)

func NewGRPCServer(node partitionNode, opts ...Option) (*grpcServer, error) {
	options := defaultOptions()
	for _, opt := range opts {
		opt(options)
	}
	if node == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	if options.maxGetBlocksBatchSize < 1 {
		return nil, errors.Wrap(errors.ErrInvalidArgument, "server-max-get-blocks-batch-size cannot be less than one")
	}
	return &grpcServer{
		node:                  node,
		maxGetBlocksBatchSize: options.maxGetBlocksBatchSize,
	}, nil
}

func (r *grpcServer) ProcessTransaction(_ context.Context, tx *txsystem.Transaction) (*emptypb.Empty, error) {
	receivedTransactionsGRPCMeter.Inc(1)
	if err := r.node.SubmitTx(tx); err != nil {
		receivedInvalidTransactionsGRPCMeter.Inc(1)
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (r *grpcServer) GetBlock(_ context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	b, err := r.node.GetBlock(req.BlockNo)
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

func (r *grpcServer) GetBlocks(_ context.Context, req *alphabill.GetBlocksRequest) (*alphabill.GetBlocksResponse, error) {
	if err := verifyRequest(req); err != nil {
		return nil, err
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
		b, err := r.node.GetBlock(blockNumber)
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
		return errors.New("block number cannot be less than one")
	}
	if req.BlockCount < 1 {
		return errors.New("block count cannot be less than one")
	}
	return nil
}
