package money

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

type (
	Options struct {
		state                    *rma.Tree
		hashAlgorithm            gocrypto.Hash
		trustBase                map[string]crypto.Verifier
		initialBill              *InitialBill
		dcMoneyAmount            uint64
		systemDescriptionRecords []*genesis.SystemDescriptionRecord
		feeCalculator            fc.FeeCalculator
	}

	Option func(*Options)
)

func DefaultOptions() *Options {
	return &Options{
		hashAlgorithm: gocrypto.SHA256,
		state:         rma.NewWithSHA256(),
		trustBase:     make(map[string]crypto.Verifier),
		dcMoneyAmount: 0,
		feeCalculator: fc.FixedFee(1),
	}
}

func WithState(state *rma.Tree) Option {
	return func(g *Options) {
		g.state = state
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

func WithInitialBill(bill *InitialBill) Option {
	return func(g *Options) {
		g.initialBill = bill
	}
}

func WithDCMoneyAmount(a uint64) Option {
	return func(g *Options) {
		g.dcMoneyAmount = a
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
