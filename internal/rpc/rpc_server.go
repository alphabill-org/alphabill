package rpc

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/errors/errstr"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type (
	rpcServer struct {
		alphabill.UnimplementedAlphabillServiceServer
		node partitionNode
	}

	partitionNode interface {
		SubmitTx(tx *txsystem.Transaction) error
		GetBlock(blockNr uint64) (*block.Block, error)
		GetLatestBlock() *block.Block
	}
)

func NewRpcServer(node partitionNode) (*rpcServer, error) {
	if node == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &rpcServer{
		node: node,
	}, nil
}

func (r *rpcServer) ProcessTransaction(ctx context.Context, tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
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

func (r *rpcServer) GetBlock(ctx context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	b, err := r.node.GetBlock(req.BlockNumber)
	if err != nil {
		return &alphabill.GetBlockResponse{ErrorMessage: err.Error()}, err
	}
	return &alphabill.GetBlockResponse{Block: b}, nil
}

func (r *rpcServer) GetMaxBlockNumber(ctx context.Context, req *alphabill.GetMaxBlockNumberRequest) (*alphabill.GetMaxBlockNumberResponse, error) {
	maxBlockNumber := r.node.GetLatestBlock().GetBlockNumber()
	return &alphabill.GetMaxBlockNumberResponse{BlockNumber: maxBlockNumber}, nil
}

func (r *rpcServer) GetBlocks(req *alphabill.GetBlocksRequest, stream alphabill.AlphabillService_GetBlocksServer) error {
	// TODO verify request
	for blockNumber := req.GetBlockNumberFrom(); blockNumber <= req.GetBlockNumberUntil(); blockNumber++ {
		b, err := r.node.GetBlock(blockNumber)
		if err != nil {
			return err
		}
		err = stream.Send(&alphabill.GetBlocksResponse{Block: b})
		if err != nil {
			return err
		}
	}
	return nil
}
