package shard

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
)

// TODO remove after task AB-160 is done
type (
	shardNode struct {
		stateProcessor txsystem.TransactionSystem
		blockStore     store.BlockStore
	}

	StateProcessor interface {
		// Execute validates and processes a transaction order.
		Execute(tx txsystem.GenericTransaction) error
		ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)
	}
)

// New create a new Shard Component.
// At the moment it only updates the state. In the future it should synchronize with other shards
// communicate with Core and Blockchain.
func New(stateProcessor txsystem.TransactionSystem, blockStore store.BlockStore) (*shardNode, error) {
	if stateProcessor == nil {
		return nil, errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &shardNode{stateProcessor, blockStore}, nil
}

func (n *shardNode) Convert(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return n.stateProcessor.ConvertTx(tx)
}

func (n *shardNode) Process(gtx txsystem.GenericTransaction) (err error) {
	stx, ok := gtx.(txsystem.GenericTransaction)
	if !ok {
		return errors.New("the transaction does not confirm to the state GenericTransaction interface")
	}
	err = n.stateProcessor.Execute(stx)
	if err != nil {
		return err
	}
	return nil
}

func (n *shardNode) GetBlock(request *alphabill.GetBlockRequest) (*alphabill.GetBlockResponse, error) {
	b, err := n.blockStore.Get(request.BlockNo)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.Errorf("block with number %v not found", request.BlockNo)
	}

	return &alphabill.GetBlockResponse{Block: &block.Block{
		BlockNumber:        b.BlockNumber,
		PreviousBlockHash:  b.PreviousBlockHash,
		Transactions:       b.Transactions,
		UnicityCertificate: b.UnicityCertificate,
	}}, nil
}

func (n *shardNode) GetMaxBlockNo(*alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	maxBlockNo, err := n.blockStore.Height()
	if err != nil {
		return nil, err
	}
	return &alphabill.GetMaxBlockNoResponse{BlockNo: maxBlockNo}, nil
}
