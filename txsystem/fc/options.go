package fc

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

type Option func(f *FeeCredit)

func WithSystemIdentifier(systemID types.SystemID) Option {
	return func(f *FeeCredit) {
		f.systemIdentifier = systemID
	}
}

func WithMoneySystemIdentifier(moneySystemID types.SystemID) Option {
	return func(f *FeeCredit) {
		f.moneySystemIdentifier = moneySystemID
	}
}

func WithState(s *state.State) Option {
	return func(f *FeeCredit) {
		f.state = s
	}
}

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(f *FeeCredit) {
		f.hashAlgorithm = hashAlgorithm
	}
}

func WithTrustBase(trustBase types.RootTrustBase) Option {
	return func(f *FeeCredit) {
		f.trustBase = trustBase
	}
}

func WithFeeCalculator(feeCalculator FeeCalculator) Option {
	return func(f *FeeCredit) {
		f.feeCalculator = feeCalculator
	}
}

func WithFeeCreditRecordUnitType(feeCreditRecordUnitType []byte) Option {
	return func(f *FeeCredit) {
		f.feeCreditRecordUnitType = feeCreditRecordUnitType
	}
}
