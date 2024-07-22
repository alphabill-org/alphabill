package permissioned

import (
	"crypto"
)

type Option func(f *FeeCreditModule)

func WithHashAlgorithm(hashAlgorithm crypto.Hash) Option {
	return func(f *FeeCreditModule) {
		f.hashAlgorithm = hashAlgorithm
	}
}
