package bsn

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
)

type (
	billShardNode struct {
		stateProcessor StateProcessor
	}
	StateProcessor interface {
		// Process validates and processes a payment order.
		Process(payment *domain.PaymentOrder) error
	}
)

var log = logger.CreateForPackage()

// New create a new Bill Shard Component.
// At the moment it only updates the state. In the future it should synchronize with other shards
// communicate with Core and Blockchain.
func New(stateProcessor StateProcessor) (*billShardNode, error) {
	if stateProcessor == nil {
		return nil, errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &billShardNode{stateProcessor}, nil
}

func (b *billShardNode) Process(payment *domain.PaymentOrder) (status string, err error) {
	err = b.stateProcessor.Process(payment)
	if err != nil {
		return "", err
	}
	return "1", nil
}

func (b *billShardNode) Status(paymentID string) (interface{}, error) {
	log.Debug("Received status request for payment ID: %s", paymentID)
	return nil, errors.ErrNotImplemented
}
