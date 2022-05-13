package rpc

import (
	"context"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
)

type (
	// Deprecated. Will be removed.
	rpcServer struct {
		alphabill.UnimplementedAlphabillServiceServer
		processor     TransactionsProcessor
		ledgerService LedgerService
	}
	// TODO rename to rpcServer
	rpcServer2 struct {
		alphabill.UnimplementedAlphabillServiceServer
		node *partition.Node
	}

	// TODO remove
	TransactionsProcessor interface {
		Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error)
		Process(gtx transaction.GenericTransaction) error
	}

	// TODO remove
	LedgerService interface {
		GetBlock(request *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error)
		GetMaxBlockNo(request *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error)
	}
)

// Deprecated. Will be removed.
func NewRpcServer(processor TransactionsProcessor, ledgerService LedgerService) (*rpcServer, error) {
	if processor == nil || ledgerService == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &rpcServer{
		processor:     processor,
		ledgerService: ledgerService,
	}, nil
}

// TODO rename to NewRpcServer
func NewRpcServer2(node *partition.Node) (*rpcServer2, error) {
	//if node == nil || ledgerService == nil {
	if node == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &rpcServer2{
		node: node,
	}, nil
}

func (r *rpcServer2) ProcessTransaction(ctx context.Context, tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	err := r.node.SubmitTx(tx)
	if err != nil {
		return &transaction.TransactionResponse{
			Ok:      false,
			Message: err.Error(),
		}, err
	}
	return &transaction.TransactionResponse{
		Ok:      true,
		Message: "",
	}, nil
}

func (r *rpcServer2) GetBlock(ctx context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	block, err := r.node.GetBlock(req.BlockNo)
	if err != nil {
		return &alphabill.GetBlockResponse{ErrorMessage: err.Error()}, err
	}
	return &alphabill.GetBlockResponse{Block: block}, nil
}

func (r *rpcServer2) GetMaxBlockNo(ctx context.Context, req *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	maxBlockNr := r.node.GetLatestBlock().GetBlockNumber()
	return &alphabill.GetMaxBlockNoResponse{BlockNo: maxBlockNr}, nil
}

func (r *rpcServer) ProcessTransaction(ctx context.Context, tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	genTx, err := r.processor.Convert(tx)
	if err != nil {
		return &transaction.TransactionResponse{
			Ok:      false,
			Message: err.Error(),
		}, err
	}
	err = r.processor.Process(genTx)
	if err != nil {
		return &transaction.TransactionResponse{
			Ok:      false,
			Message: err.Error(),
		}, err
	}
	return &transaction.TransactionResponse{
		Ok:      true,
		Message: "",
	}, nil
}

func (r *rpcServer) GetBlock(ctx context.Context, req *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	block, err := r.ledgerService.GetBlock(req)
	if err != nil {
		return &alphabill.GetBlockResponse{ErrorMessage: err.Error()}, err
	}
	return block, nil
}

func (r *rpcServer) GetMaxBlockNo(ctx context.Context, req *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	maxBlockNo, err := r.ledgerService.GetMaxBlockNo(req)
	if err != nil {
		return &alphabill.GetMaxBlockNoResponse{ErrorMessage: err.Error()}, err
	}
	return maxBlockNo, nil
}
