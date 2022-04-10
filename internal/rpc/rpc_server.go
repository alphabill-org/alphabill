package rpc

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

type (
	rpcServer struct {
		alphabill.UnimplementedAlphaBillServiceServer
		processor     TransactionsProcessor
		ledgerService LedgerService
	}

	TransactionsProcessor interface {
		Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error)
		Process(gtx transaction.GenericTransaction) error
	}

	LedgerService interface {
		GetBlock(request *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error)
		GetMaxBlockNo(request *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error)
	}
)

func NewRpcServer(processor TransactionsProcessor, ledgerService LedgerService) (*rpcServer, error) {
	if processor == nil || ledgerService == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &rpcServer{
		processor:     processor,
		ledgerService: ledgerService,
	}, nil
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
		return &alphabill.GetBlockResponse{Message: err.Error()}, err
	}
	return block, nil
}

func (r *rpcServer) GetMaxBlockNo(ctx context.Context, req *alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	maxBlockNo, err := r.ledgerService.GetMaxBlockNo(req)
	if err != nil {
		return &alphabill.GetMaxBlockNoResponse{Message: err.Error()}, err
	}
	return maxBlockNo, nil
}
