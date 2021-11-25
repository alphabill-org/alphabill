package bsn

import (
	"crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
)

type billShardNode struct {
	state *state.State
}

var log = logger.CreateForPackage()

// New creates a new empty Bill Shard Node component with one bill inside.
func New(initialBillValue int) (*billShardNode, error) {
	billState, err := state.New(crypto.SHA256)
	if err != nil {
		return nil, err
	}
	log.Info("Creating initial bill with value: ", initialBillValue)
	// TODO initialize it
	return &billShardNode{state: billState}, nil
}

func (b *billShardNode) Process(payment *state.PaymentOrder) (string, error) {
	err := b.state.Process(payment)
	if err != nil {
		return "1", nil // hardcoded status for now
	}
	return "", err
}
func (b *billShardNode) Status(paymentID string) (interface{}, error) {
	log.Debug("Received status request for payment ID: %s", paymentID)
	return nil, errors.ErrNotImplemented
}
