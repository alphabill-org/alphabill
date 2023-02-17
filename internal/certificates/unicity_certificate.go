package certificates

import (
	"bytes"
	gocrypto "crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
)

var ErrUnicityCertificateIsNil = errors.New("unicity certificate is nil")

func (x *UnicityCertificate) IsValid(verifiers map[string]crypto.Verifier, algorithm gocrypto.Hash, systemIdentifier, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrUnicityCertificateIsNil
	}
	if err := x.UnicitySeal.IsValid(verifiers); err != nil {
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
	hasher.Write(x.Bytes())
}

func (x *UnicityCertificate) Bytes() []byte {
	var b bytes.Buffer
	if x.InputRecord != nil {
		b.Write(x.InputRecord.Bytes())
	}
	if x.UnicityTreeCertificate != nil {
		b.Write(x.UnicityTreeCertificate.Bytes())
	}
	if x.UnicitySeal != nil {
		b.Write(x.UnicitySeal.Bytes())
	}
	return b.Bytes()
}

func (x *UnicityCertificate) GetRoundNumber() uint64 {
	if x != nil {
		return x.InputRecord.GetRoundNumber()
	}
	return 0
}
