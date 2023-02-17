package block

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type TxConverter interface {
	ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)
}

type TxConverterFunc func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)

func (tcf TxConverterFunc) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return tcf(tx)
}

// Hash returns the hash of the block.
func (x *Block) Hash(txConverter TxConverter, hashAlgorithm crypto.Hash) ([]byte, error) {
	b, err := x.ToGenericBlock(txConverter)
	if err != nil {
		return nil, err
	}
	return b.Hash(hashAlgorithm)
}

func (x *Block) ToGenericBlock(txConverter TxConverter) (*GenericBlock, error) {
	txs, err := protobufTxsToGeneric(x.Transactions, txConverter)
	if err != nil {
		return nil, err
	}
	return &GenericBlock{
		SystemIdentifier:   x.SystemIdentifier,
		ShardIdentifier:    x.ShardIdentifier,
		PreviousBlockHash:  x.PreviousBlockHash,
		NodeIdentifier:     x.NodeIdentifier,
		Transactions:       txs,
		UnicityCertificate: x.UnicityCertificate,
	}, nil
}
