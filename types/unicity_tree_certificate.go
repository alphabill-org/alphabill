package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/tree/imt"
)

var ErrUnicityTreeCertificateIsNil = errors.New("unicity tree certificate is nil")
var errUCIsNil = errors.New("new UC is nil")
var errLastUCIsNil = errors.New("last UC is nil")

type UnicityTreeCertificate struct {
	_                     struct{}        `cbor:",toarray"`
	SystemIdentifier      SystemID        `json:"system_identifier,omitempty"`
	SiblingHashes         []*imt.PathItem `json:"sibling_hashes,omitempty"`
	SystemDescriptionHash []byte          `json:"system_description_hash,omitempty"`
}

type treeData struct {
	Idx  SystemID
	IR   *InputRecord
	Sdrh []byte
}

func (t treeData) AddToHasher(hasher hash.Hash) {
	t.IR.AddToHasher(hasher)
	hasher.Write(t.Sdrh)
}

func (t treeData) Key() []byte {
	return t.Idx.Bytes()
}

func (x *UnicityTreeCertificate) IsValid(systemIdentifier SystemID, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrUnicityTreeCertificateIsNil
	}
	if x.SystemIdentifier != systemIdentifier {
		return fmt.Errorf("invalid system identifier: expected %s, got %s", systemIdentifier, x.SystemIdentifier)
	}
	if !bytes.Equal(systemDescriptionHash, x.SystemDescriptionHash) {
		return fmt.Errorf("invalid system description hash: expected %X, got %X", systemDescriptionHash, x.SystemDescriptionHash)
	}
	return nil
}

func (x *UnicityTreeCertificate) EvalAuthPath(ir *InputRecord, sdrh []byte, hashAlgorithm gocrypto.Hash) []byte {
	leaf := treeData{
		Idx:  x.SystemIdentifier,
		IR:   ir,
		Sdrh: sdrh,
	}
	return imt.IndexTreeOutput(x.SiblingHashes, leaf, hashAlgorithm)
}

func (x *UnicityTreeCertificate) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *UnicityTreeCertificate) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.SystemIdentifier.Bytes())
	b.Write(x.SystemDescriptionHash)
	for _, siblingHash := range x.SiblingHashes {
		b.Write(siblingHash.Key)
		b.Write(siblingHash.Hash)
	}
	return b.Bytes()
}
