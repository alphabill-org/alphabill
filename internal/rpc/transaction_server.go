package rpc

import (
	"context"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

type (
	transactionServer struct {
		transaction.UnimplementedTransactionsServer
		processor TransactionsProcessor
	}

	TransactionsProcessor interface {
		Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error)
		Process(gtx transaction.GenericTransaction) error
	}
)

func NewTransactionsServer(processor TransactionsProcessor) (*transactionServer, error) {
	if processor == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &transactionServer{
		processor: processor,
	}, nil
}

func (t *transactionServer) ProcessTransaction(ctx context.Context, tx *transaction.Transaction) (*transaction.TransactionResponse, error) {
	genTx, err := t.processor.Convert(tx)
	if err != nil {
		return &transaction.TransactionResponse{
			Ok:      false,
			Message: err.Error(),
		}, err
	}
	err = t.processor.Process(genTx)
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
