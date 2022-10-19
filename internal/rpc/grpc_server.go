package rpc

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/errors/errstr"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	grpcServer struct {
		alphabill.UnimplementedAlphabillServiceServer
		node                  partitionNode
		maxGetBlocksBatchSize uint64
	}

	partitionNode interface {
		SubmitTx(tx *txsystem.Transaction) error
		GetBlock(blockNr uint64) (*block.Block, error)
		GetLatestBlock() *block.Block
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

func (r *grpcServer) ProcessTransaction(_ context.Context, tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	err := r.node.SubmitTx(tx)
	if err != nil {
		return &txsystem.TransactionResponse{
			Ok:      false,
			Message: err.Error(),
		}, err
	}
	return &txsystem.TransactionResponse{
		Ok:      true,
		Message: "",
	}, nil
}

func (r *grpcServer) GetBlock(_ context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	b, err := r.node.GetBlock(req.BlockNo)
	if err != nil {
		return &alphabill.GetBlockResponse{ErrorMessage: err.Error()}, err
	}
	return &alphabill.GetBlockResponse{Block: b}, nil
}

func (r *grpcServer) GetMaxBlockNo(_ context.Context, req *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	maxBlockNumber := r.node.GetLatestBlock().GetBlockNumber()
	return &alphabill.GetMaxBlockNoResponse{BlockNo: maxBlockNumber}, nil
}

func (r *grpcServer) GetBlocks(_ context.Context, req *alphabill.GetBlocksRequest) (*alphabill.GetBlocksResponse, error) {
	latestBlock := r.node.GetLatestBlock()
	err := verifyRequest(req)
	if err != nil {
		return &alphabill.GetBlocksResponse{ErrorMessage: err.Error()}, err
	}
	maxBlockCount := util.Min(req.BlockCount, r.maxGetBlocksBatchSize)
	batchMaxBlockNumber := util.Min(req.BlockNumber+maxBlockCount-1, latestBlock.BlockNumber)
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
		res = append(res, b)
	}
	return &alphabill.GetBlocksResponse{Blocks: res, MaxBlockNumber: latestBlock.BlockNumber}, nil
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
