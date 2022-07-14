package money

import "github.com/alphabill-org/alphabill/internal/rma"

type (
	Options struct {
		revertibleState  *rma.Tree
		systemIdentifier []byte
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
