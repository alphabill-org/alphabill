package genesis

import (
	"bytes"
	gocrypto "crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

var ErrPartitionGenesisIsNil = errors.New("partition genesis is nil")

func (x *PartitionGenesis) IsValid(verifier crypto.Verifier, hashAlgorithm gocrypto.Hash) error {
	if x == nil {

		return ErrPartitionGenesisIsNil
	}
	if err := x.SystemDescriptionRecord.IsValid(); err != nil {
		return err
	}
	pubKeyBytes, err := verifier.MarshalPublicKey()
	if err != nil {
		return err
	}
	if !bytes.Equal(pubKeyBytes, x.TrustBase) {
		return errors.Errorf("invalid trust base. expected %X, got %X", pubKeyBytes, x.TrustBase)
	}
	sdrHash := x.SystemDescriptionRecord.Hash(hashAlgorithm)
	if err := x.Certificate.IsValid(verifier, hashAlgorithm, x.SystemDescriptionRecord.SystemIdentifier, sdrHash); err != nil {
		return err
	}
	return nil
}
