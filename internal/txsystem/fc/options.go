package fc

import (
	"crypto"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/rma"
)

type Option func(f *FeeCredit)

func WithSystemIdentifier(systemID []byte) Option {
	return func(f *FeeCredit) {
		f.systemIdentifier = systemID
	}
}

func WithMoneyTXSystemIdentifier(moneyTxSystemID []byte) Option {
	return func(f *FeeCredit) {
		f.moneyTXSystemIdentifier = moneyTxSystemID
	}
}

func WithState(state *rma.Tree) Option {
	return func(f *FeeCredit) {
		f.state = state
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

func WithLogger(logger logger.Logger) Option {
	return func(f *FeeCredit) {
		f.logger = logger
	}
}
