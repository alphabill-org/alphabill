package block

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var (
	ErrPrevBlockHashIsNil       = errors.New("previous block hash is nil")
	ErrBlockProposerIdIsMissing = errors.New("block proposer node identifier is missing")
	ErrTransactionsIsNil        = errors.New("transactions is nil")
	ErrSystemIdIsNil            = errors.New("system identifier is nil")
)

type CertificateValidator interface {
	Validate(uc *certificates.UnicityCertificate) error
}

type TxConverter interface {
	ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)
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

func (x *Block) GetRoundNumber() uint64 {
	if x != nil {
		return x.UnicityCertificate.GetRoundNumber()
	}
	return 0
}

func (x *Block) IsValid(v CertificateValidator) error {
	if x == nil {
		return ErrBlockIsNil
	}
	if x.SystemIdentifier == nil {
		return ErrSystemIdIsNil
	}
	// skip shard identifier for now, it is not used
	if x.PreviousBlockHash == nil {
		return ErrPrevBlockHashIsNil
	}
	if len(x.NodeIdentifier) == 0 {
		return ErrBlockProposerIdIsMissing
	}
	if x.Transactions == nil {
		return ErrTransactionsIsNil
	}
	if x.UnicityCertificate == nil {
		return ErrUCIsNil
	}
	if err := v.Validate(x.UnicityCertificate); err != nil {
		return fmt.Errorf("unicity certificate validation failed, %w", err)
	}
	return nil
}
