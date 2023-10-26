package genesis

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/alphabill-org/alphabill/validator/internal/util"
)

var (
	ErrSystemDescriptionIsNil = errors.New("system description record is nil")
	ErrT2TimeoutIsNil         = errors.New("t2 timeout is zero")
)

type SystemDescriptionRecord struct {
	_                struct{}       `cbor:",toarray"`
	SystemIdentifier types.SystemID `json:"system_identifier,omitempty"`
	T2Timeout        uint32         `json:"t2timeout,omitempty"`
	FeeCreditBill    *FeeCreditBill `json:"fee_credit_bill,omitempty"`
}

type FeeCreditBill struct {
	_              struct{} `cbor:",toarray"`
	UnitId         []byte   `json:"unit_id,omitempty"`
	OwnerPredicate []byte   `json:"owner_predicate,omitempty"`
}

func (x *SystemDescriptionRecord) IsValid() error {
	if x == nil {
		return ErrSystemDescriptionIsNil
	}

	if len(x.SystemIdentifier) != types.SystemIdentifierLength {
		return fmt.Errorf("invalid system identifier length: expected %v, got %v", types.SystemIdentifierLength, len(x.SystemIdentifier))
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

func (x *SystemDescriptionRecord) GetSystemIdentifier() types.SystemID {
	return x.SystemIdentifier
}
