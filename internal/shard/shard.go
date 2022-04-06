package shard

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
)

type (
	shardNode struct {
		stateProcessor StateProcessor
		txConverter    TxConverter
		blockStore     partition.BlockStore
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
func New(converter TxConverter, stateProcessor StateProcessor, blockStore partition.BlockStore) (*shardNode, error) {
	if stateProcessor == nil {
		return nil, errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	return &shardNode{stateProcessor, converter, blockStore}, nil
}

func (n *shardNode) Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	return n.txConverter.Convert(tx)
}

func (n *shardNode) Process(gtx transaction.GenericTransaction) (err error) {
	stx, ok := gtx.(txsystem.GenericTransaction)
	if !ok {
		return errors.New("the transaction does not confirm to the state GenericTransaction interface")
	}
	err = n.stateProcessor.Process(stx)
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
	uc := b.UnicityCertificate
	us := uc.UnicitySeal
	return &alphabill.GetBlockResponse{Block: &alphabill.Block{
		BlockNo:       b.TxSystemBlockNumber,
		PrevBlockHash: b.PreviousBlockHash,
		Transactions:  b.Transactions,
		UnicityCertificate: &certificates.UnicityCertificate{
			InputRecord:            uc.InputRecord,
			UnicityTreeCertificate: uc.UnicityTreeCertificate,
			UnicitySeal: &certificates.UnicitySeal{
				RootChainRoundNumber: us.RootChainRoundNumber,
				PreviousHash:         us.PreviousHash,
				Hash:                 us.Hash,
				Signature:            nil, // TODO signatures (AB-131)
			},
		},
	}}, nil
}

func (n *shardNode) GetMaxBlockNo(*alphabill.GetMaxBlockNoRequest) (*alphabill.GetMaxBlockNoResponse, error) {
	maxBlockNo, err := n.blockStore.Height()
	if err != nil {
		return nil, err
	}
	return &alphabill.GetMaxBlockNoResponse{BlockNo: maxBlockNo}, nil
}
