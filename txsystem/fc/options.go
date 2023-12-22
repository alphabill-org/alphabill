package fc

import (
	"crypto"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
)

type Option func(f *FeeCredit)

type NewFeeCreditRecordFn func(v uint64, backlink []byte, timeout uint64) unit.GenericFeeCreditRecord

func DefaultNewFeeCreditRecord(v uint64, backlink []byte, timeout uint64) unit.GenericFeeCreditRecord {
	return &unit.FeeCreditRecord{
		Balance:  v,
		Backlink: backlink,
		Timeout:  timeout,
		Locked:   0,
	}
}

func WithSystemIdentifier(systemID []byte) Option {
	return func(f *FeeCredit) {
		f.systemIdentifier = systemID
	}
}

func WithMoneySystemIdentifier(moneySystemID []byte) Option {
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

func WithFeeCreditRecordFn(newFCRFun NewFeeCreditRecordFn) Option {
	return func(f *FeeCredit) {
		f.newFeeCreditRecordFn = newFCRFun
	}
}
