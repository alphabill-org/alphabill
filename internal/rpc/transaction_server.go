package rpc

import (
	"context"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

const (
	errTransactionOrderProcessingFailed = "failed to process transaction order request"
)

type (
	transactionServer struct {
		transaction.UnimplementedTransactionsServer
		transactionProcessor TransactionProcessor
		converter            TransactionOrderConverter
	}

	TransactionProcessor interface {
		Process(transaction *domain.TransactionOrder) error
	}

	TransactionOrderConverter interface {
		ConvertPbToDomain(pbTxOrder *transaction.TransactionOrder) (*domain.TransactionOrder, error)
	}
)

func New(processor TransactionProcessor, customOpts ...Option) (*transactionServer, error) {
	if processor == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}

	opts := Options{
		converter: &defaultConverter{},
	}
	for _, o := range customOpts {
		o(&opts)
	}

	return &transactionServer{
			transactionProcessor: processor,
			converter:            opts.converter},
		nil
}

func (ts *transactionServer) ProcessTransaction(_ context.Context, req *transaction.TransactionOrder) (*transaction.TransactionResponse, error) {
	tx, err := ts.converter.ConvertPbToDomain(req)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert protobuf request to domain type")
	}
	err = ts.transactionProcessor.Process(tx)
	if err != nil {
		return nil, errors.Wrap(err, errTransactionOrderProcessingFailed)
	}
	return &transaction.TransactionResponse{Ok: true}, nil
}
