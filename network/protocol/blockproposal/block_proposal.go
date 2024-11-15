package blockproposal

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

var (
	ErrBlockProposalIsNil         = errors.New("block proposal is nil")
	ErrTrustBaseIsNil             = errors.New("trust base is nil")
	ErrSignerIsNil                = errors.New("signer is nil")
	ErrNodeVerifierIsNil          = errors.New("node signature verifier is nil")
	ErrInvalidPartitionIdentifier = errors.New("invalid partition identifier")
	errBlockProposerIDMissing     = errors.New("block proposer id is missing")
)

type BlockProposal struct {
	_                  struct{} `cbor:",toarray"`
	Partition          types.PartitionID
	Shard              types.ShardID
	NodeIdentifier     string
	UnicityCertificate *types.UnicityCertificate
	Technical          certification.TechnicalRecord
	Transactions       []*types.TransactionRecord
	Signature          []byte
}

func (x *BlockProposal) IsValid(nodeSignatureVerifier crypto.Verifier, tb types.RootTrustBase, algorithm gocrypto.Hash, partitionIdentifier types.PartitionID, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrBlockProposalIsNil
	}
	if nodeSignatureVerifier == nil {
		return ErrNodeVerifierIsNil
	}
	if len(x.NodeIdentifier) == 0 {
		return errBlockProposerIDMissing
	}
	if tb == nil {
		return ErrTrustBaseIsNil
	}
	if partitionIdentifier != x.Partition {
		return fmt.Errorf("%w, expected %s, got %s", ErrInvalidPartitionIdentifier, partitionIdentifier, x.Partition)
	}
	if err := x.UnicityCertificate.Verify(tb, algorithm, partitionIdentifier, systemDescriptionHash); err != nil {
		return err
	}
	if err := x.Technical.IsValid(); err != nil {
		return fmt.Errorf("invalid TechnicalRecord: %w", err)
	}
	if err := x.Technical.HashMatches(x.UnicityCertificate.TRHash); err != nil {
		return fmt.Errorf("comparing TechnicalRecord hash to UC.TRHash: %w", err)
	}
	return x.Verify(algorithm, nodeSignatureVerifier)
}

func (x *BlockProposal) Hash(algorithm gocrypto.Hash) ([]byte, error) {
	hasher := algorithm.New()
	hasher.Write(x.Partition.Bytes())
	hasher.Write([]byte(x.NodeIdentifier))

	ucBytes, err := types.Cbor.Marshal(x.UnicityCertificate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal unicity certificate: %w", err)
	}
	hasher.Write(ucBytes)
	for _, tx := range x.Transactions {
		txBytes, err := tx.Bytes()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal transaction record: %w", err)
		}
		hasher.Write(txBytes)
	}
	return hasher.Sum(nil), nil
}

func (x *BlockProposal) Sign(algorithm gocrypto.Hash, signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	hash, err := x.Hash(algorithm)
	if err != nil {
		return err
	}
	x.Signature, err = signer.SignHash(hash)
	if err != nil {
		return err
	}
	return nil
}

func (x *BlockProposal) Verify(algorithm gocrypto.Hash, nodeSignatureVerifier crypto.Verifier) error {
	if nodeSignatureVerifier == nil {
		return ErrNodeVerifierIsNil
	}
	hash, err := x.Hash(algorithm)
	if err != nil {
		return err
	}
	return nodeSignatureVerifier.VerifyHash(x.Signature, hash)
}
