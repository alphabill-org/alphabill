package tokens

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/money"
)

var DefaultSystemIdentifier = []byte{0, 0, 0, 2}

type (
	Options struct {
		systemIdentifier        []byte
		moneyTXSystemIdentifier []byte
		hashAlgorithm           gocrypto.Hash
		trustBase               map[string]crypto.Verifier
		state                   *state.State
		feeCalculator           fc.FeeCalculator
	}

	Option func(*Options)
)

func defaultOptions() *Options {
	return &Options{
		systemIdentifier:        DefaultSystemIdentifier,
		moneyTXSystemIdentifier: money.DefaultSystemIdentifier,
		hashAlgorithm:           gocrypto.SHA256,
		feeCalculator:           fc.FixedFee(1),
		trustBase:               map[string]crypto.Verifier{},
	}
}

func WithState(s *state.State) Option {
	return func(c *Options) {
		c.state = s
	}
}

func WithSystemIdentifier(systemIdentifier []byte) Option {
	return func(c *Options) {
		c.systemIdentifier = systemIdentifier
	}
}

func WithMoneyTXSystemIdentifier(moneySystemIdentifier []byte) Option {
	return func(c *Options) {
		c.moneyTXSystemIdentifier = moneySystemIdentifier
	}
}

func WithFeeCalculator(calc fc.FeeCalculator) Option {
	return func(g *Options) {
		g.feeCalculator = calc
	}
}

func WithHashAlgorithm(algorithm gocrypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}

func WithTrustBase(trustBase map[string]crypto.Verifier) Option {
	return func(c *Options) {
		c.trustBase = trustBase
	}
}
