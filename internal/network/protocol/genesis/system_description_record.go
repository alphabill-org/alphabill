package genesis

import (
	gocrypto "crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/errors"

	"github.com/alphabill-org/alphabill/internal/util"
)

const systemIdentifierLength = 4

var (
	ErrSystemDescriptionIsNil = errors.New("system description record is nil")
	ErrT2TimeoutIsNil         = errors.New("t2 timeout is zero")
)

func (x *SystemDescriptionRecord) IsValid() error {
	if x == nil {
		return ErrSystemDescriptionIsNil
	}

	if len(x.SystemIdentifier) != systemIdentifierLength {
		return errors.Errorf("invalid system identifier length: expected %v, got %v", systemIdentifierLength, len(x.SystemIdentifier))
	}
	if x.T2Timeout == 0 {
		return ErrT2TimeoutIsNil
	}
	return nil
}

func (x *SystemDescriptionRecord) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	hasher.Write(util.Uint64ToBytes(uint64(x.T2Timeout)))
}

func (x *SystemDescriptionRecord) Hash(hashAlgorithm gocrypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	x.AddToHasher(hasher)
	return hasher.Sum(nil)
}

func (x *SystemDescriptionRecord) GetSystemIdentifierString() string {
	return string(x.SystemIdentifier)
}
