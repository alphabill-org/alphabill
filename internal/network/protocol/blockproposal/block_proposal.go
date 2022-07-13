package blockproposal

import (
	"bytes"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/mt"
)

var (
	ErrBlockProposalIsNil      = errors.New("block proposal is nil")
	ErrTrustBaseIsNil          = errors.New("trust base is nil")
	ErrSignerIsNil             = errors.New("signer is nil")
	ErrNodeVerifierIsNil       = errors.New("node signature verifier is nil")
	ErrInvalidSystemIdentifier = errors.New("invalid system identifier")
)

func (x *BlockProposal) IsValid(nodeSignatureVerifier crypto.Verifier, ucTrustBase crypto.Verifier, algorithm gocrypto.Hash, systemIdentifier []byte, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrBlockProposalIsNil
	}
	if nodeSignatureVerifier == nil {
		return ErrNodeVerifierIsNil
	}
	if ucTrustBase == nil {
		return ErrTrustBaseIsNil
	}
	if !bytes.Equal(systemIdentifier, x.SystemIdentifier) {
		return errors.Wrapf(ErrInvalidSystemIdentifier, "expected %X, got %X", systemIdentifier, x.SystemIdentifier)
	}
	if err := x.UnicityCertificate.IsValid(ucTrustBase, algorithm, systemIdentifier, systemDescriptionHash); err != nil {
		return err
	}
	return x.Verify(algorithm, nodeSignatureVerifier)
}

func (x *BlockProposal) Hash(algorithm gocrypto.Hash) ([]byte, error) {
	hasher := algorithm.New()
	hasher.Write(x.SystemIdentifier)
	hasher.Write([]byte(x.NodeIdentifier))
	x.UnicityCertificate.AddToHasher(hasher)
	if len(x.Transactions) > 0 {
		txs := make([]mt.Data, len(x.Transactions))
		for i, tx := range x.Transactions {
			txBytes, err := tx.Bytes()
			if err != nil {
				return nil, err
			}
			txs[i] = &byteHasher{val: txBytes}
		}
		// build merkle tree of transactions
		merkleTree, err := mt.New(algorithm, txs)
		if err != nil {
			return nil, err
		}
		// add merkle tree root hash to block hasher
		hasher.Write(merkleTree.GetRootHash())
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

// byteHasher helper struct to satisfy mt.Data interface
type byteHasher struct {
	val []byte
}

func (h *byteHasher) Hash(hashAlgorithm gocrypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(h.val)
	return hasher.Sum(nil)
}