package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/common/smt"
)

var ErrUnicityTreeCertificateIsNil = errors.New("unicity tree certificate is nil")
var errUCIsNil = errors.New("new UC is nil")
var errLastUCIsNil = errors.New("last UC is nil")

type UnicityTreeCertificate struct {
	_                     struct{} `cbor:",toarray"`
	SystemIdentifier      SystemID `json:"system_identifier,omitempty"`
	SiblingHashes         [][]byte `json:"sibling_hashes,omitempty"`
	SystemDescriptionHash []byte   `json:"system_description_hash,omitempty"`
}

func (x *UnicityTreeCertificate) IsValid(systemIdentifier, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrUnicityTreeCertificateIsNil
	}
	if !bytes.Equal(x.SystemIdentifier, systemIdentifier) {
		return fmt.Errorf("invalid system identifier: expected %X, got %X", systemIdentifier, x.SystemIdentifier)
	}
	if !bytes.Equal(systemDescriptionHash, x.SystemDescriptionHash) {
		return fmt.Errorf("invalid system description hash: expected %X, got %X", systemDescriptionHash, x.SystemDescriptionHash)
	}

	siblingHashesLength := len(systemIdentifier) * 8 // bits in system identifier
	if c := len(x.SiblingHashes); c != siblingHashesLength {
		return fmt.Errorf("invalid count of sibling hashes: expected %v, got %v", siblingHashesLength, c)
	}
	return nil
}

func (x *UnicityTreeCertificate) GetAuthPath(leafHash []byte, hashAlgorithm gocrypto.Hash) ([]byte, error) {
	return smt.CalculatePathRoot(x.SiblingHashes, leafHash, x.SystemIdentifier, hashAlgorithm)
}

func (x *UnicityTreeCertificate) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *UnicityTreeCertificate) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.SystemIdentifier)
	b.Write(x.SystemDescriptionHash)
	for _, siblingHash := range x.SiblingHashes {
		b.Write(siblingHash)
	}
	return b.Bytes()
}
