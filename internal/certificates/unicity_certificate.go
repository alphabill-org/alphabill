package certificates

import (
	"bytes"
	gocrypto "crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var ErrUnicityCertificateIsNil = errors.New("unicity certificate is nil")

func (x *UnicityCertificate) IsValid(verifier crypto.Verifier, algorithm gocrypto.Hash, systemIdentifier, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrUnicityCertificateIsNil
	}
	if err := x.UnicitySeal.IsValid(verifier); err != nil {
		return err
	}
	if err := x.InputRecord.IsValid(); err != nil {
		return err
	}
	if err := x.UnicityTreeCertificate.IsValid(systemIdentifier, systemDescriptionHash); err != nil {
		return err
	}
	hasher := algorithm.New()
	x.InputRecord.AddToHasher(hasher)
	hasher.Write(x.UnicityTreeCertificate.SystemDescriptionHash)
	treeRoot, err := x.UnicityTreeCertificate.GetAuthPath(hasher.Sum(nil), algorithm)
	if err != nil {
		return err
	}
	rootHash := x.UnicitySeal.Hash
	if !bytes.Equal(treeRoot, rootHash) {
		return errors.Errorf("unicity seal hash %X does not match with the root hash of the unicity tree %X", rootHash, treeRoot)
	}
	return nil
}

func (x *UnicityCertificate) AddToHasher(hasher hash.Hash) {
	x.InputRecord.AddToHasher(hasher)
	x.UnicityTreeCertificate.AddToHasher(hasher)
	x.UnicitySeal.AddToHasher(hasher)
}
