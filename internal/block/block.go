package block

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var (
	errPrevBlockHashIsNil       = errors.New("previous block hash is nil")
	errBlockProposerIdIsMissing = errors.New("block proposer node identifier is missing")
	errTransactionsIsNil        = errors.New("transactions is nil")
	errSystemIdIsNil            = errors.New("system identifier is nil")
)

type UCValidator interface {
	Validate(uc *certificates.UnicityCertificate) error
}

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
		return nil, fmt.Errorf("block conversion to generic type failed, %w", err)
	}
	return b.Hash(hashAlgorithm)
}

func (x *Block) ToGenericBlock(txConverter TxConverter) (*GenericBlock, error) {
	txs, err := ProtobufTxsToGeneric(x.Transactions, txConverter)
	if err != nil {
		return nil, fmt.Errorf("trancation conversion failed, %w", err)
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

func (x *Block) GetPrimaryProof(unitID []byte, txc TxConverter, hashAlgorithm crypto.Hash) (*BlockProof, error) {
	block, err := x.ToGenericBlock(txc)
	if err != nil {
		return nil, fmt.Errorf("block converstion to genetic type failed, %w", err)
	}
	return NewPrimaryProof(block, unitID, hashAlgorithm)
}

func (x *Block) GetRoundNumber() uint64 {
	if x != nil {
		return x.UnicityCertificate.GetRoundNumber()
	}
	return 0
}

func (x *Block) IsValid(v UCValidator) error {
	if x == nil {
		return ErrBlockIsNil
	}
	if len(x.SystemIdentifier) != 4 {
		return errSystemIdIsNil
	}
	// skip shard identifier for now, it is not used
	if x.PreviousBlockHash == nil {
		return errPrevBlockHashIsNil
	}
	if len(x.NodeIdentifier) == 0 {
		return errBlockProposerIdIsMissing
	}
	if x.Transactions == nil {
		return errTransactionsIsNil
	}
	if x.UnicityCertificate == nil {
		return ErrUCIsNil
	}
	if err := v.Validate(x.UnicityCertificate); err != nil {
		return fmt.Errorf("unicity certificate validation failed, %w", err)
	}
	return nil
}
