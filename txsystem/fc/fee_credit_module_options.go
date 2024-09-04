package fc

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

type Option func(f *FeeCreditModule)

func WithSystemID(systemID types.SystemID) Option {
	return func(f *FeeCreditModule) {
		f.systemIdentifier = systemID
	}
}

func WithMoneySystemID(moneySystemID types.SystemID) Option {
	return func(f *FeeCreditModule) {
		f.moneySystemIdentifier = moneySystemID
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
