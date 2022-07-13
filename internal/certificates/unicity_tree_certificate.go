package certificates

import (
	"bytes"
	gocrypto "crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/smt"

	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrUnicityTreeCertificateIsNil = errors.New("unicity tree certificate is nil")

func (x *UnicityTreeCertificate) IsValid(systemIdentifier, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrUnicityTreeCertificateIsNil
	}
	if !bytes.Equal(x.SystemIdentifier, systemIdentifier) {
		return errors.Errorf("invalid system identifier: expected %X, got %X", systemIdentifier, x.SystemIdentifier)
	}
	if !bytes.Equal(systemDescriptionHash, x.SystemDescriptionHash) {
		return errors.Errorf("invalid system description hash: expected %X, got %X", systemDescriptionHash, x.SystemDescriptionHash)
	}

	siblingHashesLength := len(systemIdentifier) * 8 // bits in system identifier
	if c := len(x.SiblingHashes); c != siblingHashesLength {
		return errors.Errorf("invalid count of sibling hashes: expected %v, got %v", siblingHashesLength, c)
	}
	return nil
}

func (x *UnicityTreeCertificate) GetAuthPath(leafHash []byte, hashAlgorithm gocrypto.Hash) ([]byte, error) {
	return smt.CalculatePathRoot(x.SiblingHashes, leafHash, x.SystemIdentifier, hashAlgorithm)
}

func (x *UnicityTreeCertificate) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	hasher.Write(x.SystemDescriptionHash)
	for _, siblingHash := range x.SiblingHashes {
		hasher.Write(siblingHash)
	}
}
