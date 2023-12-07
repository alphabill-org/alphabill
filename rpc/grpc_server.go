package rpc

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type (
	grpcServer struct {
		alphabill.UnimplementedAlphabillServiceServer
		node                  partitionNode
		maxGetBlocksBatchSize uint64

		txCnt metric.Int64Counter
	}

	partitionNode interface {
		SubmitTx(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetBlock(ctx context.Context, blockNr uint64) (*types.Block, error)
		GetLatestBlock() (*types.Block, error)
		GetTransactionRecord(ctx context.Context, hash []byte) (*types.TransactionRecord, *types.TxProof, error)
		GetLatestRoundNumber(ctx context.Context) (uint64, error)
		SystemIdentifier() []byte
		GetUnitState(unitID []byte, returnProof bool, returnData bool) (*types.UnitDataAndProof, error)
		WriteStateFile(writer io.Writer) error
	}
)

func NewGRPCServer(node partitionNode, obs Observability, opts ...Option) (*grpcServer, error) {
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

	mtr := obs.Meter(MetricsScopeGRPCAPI)
	txCnt, err := mtr.Int64Counter("tx.count", metric.WithUnit("{transaction}"), metric.WithDescription("Number of transactions submitted"))
	if err != nil {
		return nil, fmt.Errorf("creating tx.count metric: %w", err)
	}

	return &grpcServer{
		node:                  node,
		maxGetBlocksBatchSize: options.maxGetBlocksBatchSize,
		txCnt:                 txCnt,
	}, nil
}

func (r *grpcServer) ProcessTransaction(ctx context.Context, tx *alphabill.Transaction) (*emptypb.Empty, error) {
	txo := &types.TransactionOrder{}
	if err := cbor.Unmarshal(tx.Order, txo); err != nil {
		r.txCnt.Add(ctx, 1, metric.WithAttributes(attribute.String("status", "err.cbor")))
		return nil, err
	}

	_, err := r.node.SubmitTx(ctx, txo)
	r.txCnt.Add(ctx, 1, metric.WithAttributes(attribute.String("tx", txo.PayloadType()), attribute.String("status", statusCodeOfTxError(err))))
	if err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (r *grpcServer) GetBlock(ctx context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	b, err := r.node.GetBlock(ctx, req.BlockNo)
	if err != nil {
		return nil, err
	}
	bytes, err := cbor.Marshal(b)
	if err != nil {
		return nil, err
	}
	return &alphabill.GetBlockResponse{Block: bytes}, nil
}

func (r *grpcServer) GetRoundNumber(ctx context.Context, _ *emptypb.Empty) (*alphabill.GetRoundNumberResponse, error) {
	latestRn, err := r.node.GetLatestRoundNumber(ctx)
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
	latestRn, err := r.node.GetLatestRoundNumber(ctx)
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

func InstrumentMetricsUnaryServerInterceptor(mtr metric.Meter, log *slog.Logger) grpc.UnaryServerInterceptor {
	callCnt, err := mtr.Int64Counter("calls", metric.WithDescription("How many times the endpoint has been called"))
	if err != nil {
		log.Error("creating calls counter", logger.Error(err))
		return passthroughUnaryServerInterceptor
	}
	callDur, err := mtr.Float64Histogram("duration",
		metric.WithDescription("How long it took to serve the request"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(100e-6, 200e-6, 400e-6, 800e-6, 0.0016, 0.01, 0.05, 0.1))

	if err != nil {
		log.Error("creating duration histogram", logger.Error(err))
		return passthroughUnaryServerInterceptor
	}

	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		start := time.Now()
		rsp, err := handler(ctx, req)

		s, _ := status.FromError(err)
		attrSet := attribute.NewSet(semconv.RPCMethod(info.FullMethod), semconv.RPCGRPCStatusCodeKey.Int(int(s.Code())))
		callCnt.Add(ctx, 1, metric.WithAttributeSet(attrSet))
		callDur.Record(ctx, time.Since(start).Seconds(), metric.WithAttributeSet(attrSet))

		return rsp, err
	}
}

func passthroughUnaryServerInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	return handler(ctx, req)
}
