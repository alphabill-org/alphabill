package money

import (
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
)

type (
	Options struct {
		revertibleState  *rma.Tree
		systemIdentifier []byte
		trustBase        map[string]crypto.Verifier
	}

	Option                func(*Options)
	allMoneySchemeOptions struct{}
)

var (
	SchemeOpts = &allMoneySchemeOptions{}
)

// RevertibleState sets the revertible state used. Otherwise, default implementation is used.
func (o *allMoneySchemeOptions) RevertibleState(rt *rma.Tree) Option {
	return func(options *Options) {
		options.revertibleState = rt
	}
}

// SystemIdentifier sets the system identifier.
func (o *allMoneySchemeOptions) SystemIdentifier(systemIdentifier []byte) Option {
	return func(options *Options) {
		options.systemIdentifier = systemIdentifier
	}
}

// TrustBase sets the trust base (root public key info)
func (o *allMoneySchemeOptions) TrustBase(trust map[string]crypto.Verifier) Option {
	return func(options *Options) {
		options.trustBase = trust
	}
}
