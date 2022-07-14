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
	block, err := r.node.GetBlock(req.BlockNo)
	if err != nil {
		return &alphabill.GetBlockResponse{ErrorMessage: err.Error()}, err
	}
	return &alphabill.GetBlockResponse{Block: block}, nil
}

func (r *rpcServer) GetMaxBlockNo(ctx context.Context, req *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	maxBlockNr := r.node.GetLatestBlock().GetBlockNumber()
	return &alphabill.GetMaxBlockNoResponse{BlockNo: maxBlockNr}, nil
}
