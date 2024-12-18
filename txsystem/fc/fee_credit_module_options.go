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

func WithFeeCreditRecordUnitType(feeCreditRecordUnitType uint32) Option {
	return func(f *FeeCreditModule) {
		f.feeCreditRecordUnitType = feeCreditRecordUnitType
	}
}
