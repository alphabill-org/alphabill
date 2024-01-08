package money

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/types"
)

const DefaultSystemIdentifier types.SystemID = 0x00000001

type (
	Options struct {
		systemIdentifier         types.SystemID
		state                    *state.State
		hashAlgorithm            gocrypto.Hash
		trustBase                map[string]crypto.Verifier
		systemDescriptionRecords []*genesis.SystemDescriptionRecord
		feeCalculator            fc.FeeCalculator
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		systemIdentifier: DefaultSystemIdentifier,
		hashAlgorithm:    gocrypto.SHA256,
		trustBase:        make(map[string]crypto.Verifier),
		feeCalculator:    fc.FixedFee(1),
	}
}

func WithSystemIdentifier(systemIdentifier types.SystemID) Option {
	return func(g *Options) {
		g.systemIdentifier = systemIdentifier
	}
}

func WithState(s *state.State) Option {
	return func(g *Options) {
		g.state = s
	}
}

func WithTrustBase(trust map[string]crypto.Verifier) Option {
	return func(options *Options) {
		options.trustBase = trust
	}
}

func WithHashAlgorithm(hashAlgorithm gocrypto.Hash) Option {
	return func(g *Options) {
		g.hashAlgorithm = hashAlgorithm
	}
}

func WithSystemDescriptionRecords(records []*genesis.SystemDescriptionRecord) Option {
	return func(g *Options) {
		g.systemDescriptionRecords = records
	}
}

func WithFeeCalculator(calc fc.FeeCalculator) Option {
	return func(g *Options) {
		g.feeCalculator = calc
	}
}
