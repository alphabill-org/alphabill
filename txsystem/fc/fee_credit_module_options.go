package fc

import (
	"crypto"
)

type Option func(f *FeeCreditModule)

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
