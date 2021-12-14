package rpc

import (
	"context"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/payment"
)

const (
	errPaymentOrderProcessingFailed  = "failed to process payment order request"
	errPaymentStatusProcessingFailed = "failed to process payment status request"
)

// Deprecated: paymentsServer in favor of transactions server
type paymentsServer struct {
	payment.UnimplementedPaymentsServer
	paymentProcessor PaymentProcessor
}

type PaymentProcessor interface {
	Process(payment *domain.PaymentOrder) (string, error)
	Status(paymentID string) (interface{}, error)
}

func (pss *paymentsServer) MakePayment(_ context.Context, req *payment.PaymentRequest) (*payment.PaymentResponse, error) {
	paymentOrder := &domain.PaymentOrder{
		Type:              domain.PaymentType(req.PaymentType),
		BillID:            req.BillId,
		Amount:            req.Amount,
		Backlink:          req.Backlink,
		PredicateArgument: req.PredicateArgument,
		PayeePredicate:    req.PayeePredicate,
	}
	paymentID, err := pss.paymentProcessor.Process(paymentOrder)
	if err != nil {
		return nil, errors.Wrap(err, errPaymentOrderProcessingFailed)
	}
	return &payment.PaymentResponse{PaymentId: paymentID}, nil
}

func (pss *paymentsServer) PaymentStatus(_ context.Context, req *payment.PaymentStatusRequest) (*payment.PaymentStatusResponse, error) {
	s, err := pss.paymentProcessor.Status(req.PaymentId)
	if err != nil {
		return nil, errors.Wrap(err, errPaymentStatusProcessingFailed)
	}
	// TODO PaymentStatusResponse
	return &payment.PaymentStatusResponse{Status: s != nil}, nil
}

// Deprecated: NewPaymentServer in favor of transactions server
func NewPaymentServer(processor PaymentProcessor) (*paymentsServer, error) {
	if processor == nil {
		return nil, errors.Wrap(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &paymentsServer{paymentProcessor: processor}, nil
}
