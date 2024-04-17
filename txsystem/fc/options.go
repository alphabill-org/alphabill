package fc

import (
	"crypto"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill-go-sdk/types"
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

func WithTrustBase(trustBase map[string]abcrypto.Verifier) Option {
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
