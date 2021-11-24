package rpc

import (
	"context"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/payment"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
)

type PaymentsServer struct {
	payment.UnimplementedPaymentsServer
	paymentProcessor PaymentProcessor
}

type PaymentProcessor interface {
	Process(payment *state.PaymentOrder) (string, error)
	Status(paymentID string) (interface{}, error)
}

func (pss *PaymentsServer) MakePayment(_ context.Context, req *payment.PaymentRequest) (*payment.PaymentResponse, error) {
	paymentOrder := &state.PaymentOrder{
		Type:              state.PaymentType(req.PaymentType),
		BillID:            req.BillId,
		Amount:            req.Amount,
		Backlink:          req.Backlink,
		PredicateArgument: req.PredicateArgument,
		PayeePredicate:    req.PredicateArgument,
	}
	paymentID, err := pss.paymentProcessor.Process(paymentOrder)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process payment order request")
	}
	return &payment.PaymentResponse{PaymentId: paymentID}, nil
}

func (pss *PaymentsServer) PaymentStatus(_ context.Context, req *payment.PaymentStatusRequest) (*payment.PaymentStatusResponse, error) {
	s, err := pss.paymentProcessor.Status(req.PaymentId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to process payment status request")
	}
	// TODO PaymentStatusResponse
	return &payment.PaymentStatusResponse{Status: s != nil}, nil
}

func New(processor PaymentProcessor) (*PaymentsServer, error) {
	if processor == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &PaymentsServer{paymentProcessor: processor}, nil
}
