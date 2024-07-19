package permissioned

import (
	"crypto"
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrMissingSystemIdentifier        = errors.New("system identifier is missing")
	ErrStateIsNil                     = errors.New("state is nil")
	ErrMissingFeeCreditRecordUnitType = errors.New("fee credit record unit type is missing")
	ErrMissingAdminOwnerCondition     = errors.New("admin owner condition is missing")
)

type Option func(f *FeeCreditModule)

func WithSystemIdentifier(systemID types.SystemID) Option {
	return func(f *FeeCreditModule) {
		f.systemIdentifier = systemID
	}
}

func WithState(s *state.State) Option {
	return func(f *FeeCreditModule) {
		f.state = s
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(f *FeeCreditModule) {
		f.hashAlgorithm = hashAlgorithm
	}
}

func WithFeeCreditRecordUnitType(feeCreditRecordUnitType []byte) Option {
	return func(f *FeeCreditModule) {
		f.feeCreditRecordUnitType = feeCreditRecordUnitType
	}
}

func WithAdminOwnerCondition(adminOwnerCondition types.PredicateBytes) Option {
	return func(f *FeeCreditModule) {
		f.adminOwnerCondition = adminOwnerCondition
	}
}
