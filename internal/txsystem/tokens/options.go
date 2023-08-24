package tokens

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
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

func defaultOptions() (*Options, error) {
	return &Options{
		systemIdentifier:        DefaultSystemIdentifier,
		moneyTXSystemIdentifier: money.DefaultSystemIdentifier,
		hashAlgorithm:           gocrypto.SHA256,
		state:                   state.NewEmptyState(),
		feeCalculator:           fc.FixedFee(1),
		trustBase:               map[string]crypto.Verifier{},
	}, nil
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
