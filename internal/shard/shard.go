package shard

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
)

type (
	shardNode struct {
		stateProcessor StateProcessor
		txConverter    TxConverter
	}
	StateProcessor interface {
		// Process validates and processes a transaction order.
		Process(tx txsystem.GenericTransaction) error
	}
	TxConverter interface {
		Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error)
	}
)

// New create a new Shard Component.
// At the moment it only updates the state. In the future it should synchronize with other shards
// communicate with Core and Blockchain.
func New(converter TxConverter, stateProcessor StateProcessor) (*shardNode, error) {
	if stateProcessor == nil {
		return nil, errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &shardNode{stateProcessor, converter}, nil
}

func (b *shardNode) Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	return b.txConverter.Convert(tx)
}

func (b *shardNode) Process(gtx transaction.GenericTransaction) (err error) {
	stx, ok := gtx.(txsystem.GenericTransaction)
	if !ok {
		return errors.New("the transaction does not confirm to the state GenericTransaction interface")
	}
	err = b.stateProcessor.Process(stx)
	if err != nil {
		return err
	}
	return nil
}
