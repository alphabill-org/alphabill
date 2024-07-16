package permissioned

import (
	"crypto"
	"errors"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

var (
	ErrSystemIdentifierMissing = errors.New("system identifier is missing")
	ErrStateIsNil              = errors.New("state is nil")
	ErrTrustBaseIsNil          = errors.New("trust base is nil")
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

func WithTrustBase(trustBase types.RootTrustBase) Option {
	return func(f *FeeCreditModule) {
		f.trustBase = trustBase
	}
}

func WithFeeCreditRecordUnitType(feeCreditRecordUnitType []byte) Option {
	return func(f *FeeCreditModule) {
		f.feeCreditRecordUnitType = feeCreditRecordUnitType
	}
}
