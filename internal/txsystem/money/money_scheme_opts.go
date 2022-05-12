package money

type (
	Options struct {
		revertibleState RevertibleState
	}

	Option                func(*Options)
	allMoneySchemeOptions struct{}
)

var (
	SchemeOpts = &allMoneySchemeOptions{}
)

// RevertibleState sets the revertible state used. Otherwise, default implementation is used.
func (o *allMoneySchemeOptions) RevertibleState(rt RevertibleState) Option {
	return func(options *Options) {
		options.revertibleState = rt
	}
}
