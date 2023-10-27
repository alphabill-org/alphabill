package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/validator/pkg/metrics"
	alphabill2 "github.com/alphabill-org/alphabill/validator/pkg/rpc/alphabill"
	"github.com/fxamacker/cbor/v2"
	"google.golang.org/protobuf/types/known/emptypb"
)

var receivedTransactionsGRPCMeter = metrics.GetOrRegisterCounter("transactions/grpc/received")
var receivedInvalidTransactionsGRPCMeter = metrics.GetOrRegisterCounter("transactions/grpc/invalid")

type (
	grpcServer struct {
		alphabill2.UnimplementedAlphabillServiceServer
		node                  partitionNode
		maxGetBlocksBatchSize uint64
	}

	partitionNode interface {
		SubmitTx(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetBlock(ctx context.Context, blockNr uint64) (*types.Block, error)
		GetLatestBlock() (*types.Block, error)
		GetTransactionRecord(ctx context.Context, hash []byte) (*types.TransactionRecord, *types.TxProof, error)
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

func (r *grpcServer) ProcessTransaction(ctx context.Context, tx *alphabill2.Transaction) (*emptypb.Empty, error) {
	receivedTransactionsGRPCMeter.Inc(1)
	txo := &types.TransactionOrder{}
	if err := cbor.Unmarshal(tx.Order, txo); err != nil {
		receivedInvalidTransactionsGRPCMeter.Inc(1)
		return nil, err
	}

	if _, err := r.node.SubmitTx(ctx, txo); err != nil {
		receivedInvalidTransactionsGRPCMeter.Inc(1)
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (r *grpcServer) GetBlock(ctx context.Context, req *alphabill2.GetBlockRequest) (*alphabill2.GetBlockResponse, error) {
	b, err := r.node.GetBlock(ctx, req.BlockNo)
	if err != nil {
		return nil, err
	}
	bytes, err := cbor.Marshal(b)
	if err != nil {
		return nil, err
	}
	return &alphabill2.GetBlockResponse{Block: bytes}, nil
}

func (r *grpcServer) GetRoundNumber(_ context.Context, _ *emptypb.Empty) (*alphabill2.GetRoundNumberResponse, error) {
	latestRn, err := r.node.GetLatestRoundNumber()
	if err != nil {
		return nil, err
	}
	return &alphabill2.GetRoundNumberResponse{RoundNumber: latestRn}, nil
}

func (r *grpcServer) GetBlocks(ctx context.Context, req *alphabill2.GetBlocksRequest) (*alphabill2.GetBlocksResponse, error) {
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

	maxBlockCount := min(req.BlockCount, r.maxGetBlocksBatchSize)
	batchMaxBlockNumber := min(req.BlockNumber+maxBlockCount-1, latestRn)
	batchSize := uint64(0)
	if batchMaxBlockNumber >= req.BlockNumber {
		batchSize = batchMaxBlockNumber - req.BlockNumber + 1
	}
	res := make([][]byte, 0, batchSize)
	for blockNumber := req.BlockNumber; blockNumber <= batchMaxBlockNumber; blockNumber++ {
		b, err := r.node.GetBlock(ctx, blockNumber)
		if err != nil {
			return nil, err
		}
		if b == nil {
			continue
		}
		bytes, err := cbor.Marshal(b)
		if err != nil {
			return nil, err
		}
		res = append(res, bytes)
	}
	return &alphabill2.GetBlocksResponse{Blocks: res, MaxBlockNumber: latestBlock.UnicityCertificate.InputRecord.RoundNumber, MaxRoundNumber: latestRn, BatchMaxBlockNumber: batchMaxBlockNumber}, nil
}

func verifyRequest(req *alphabill2.GetBlocksRequest) error {
	if req.BlockNumber < 1 {
		return fmt.Errorf("block number cannot be less than one, got %d", req.BlockNumber)
	}
	if req.BlockCount < 1 {
		return fmt.Errorf("block count cannot be less than one, got %d", req.BlockCount)
	}
	return nil
}
