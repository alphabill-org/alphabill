package money

type (
	MoneySchemeOptions struct {
		revertibleState RevertibleState
	}

	MoneySchemeOption     func(*MoneySchemeOptions)
	allMoneySchemeOptions struct{}
)

var (
	MoneySchemeOpts = &allMoneySchemeOptions{}
)

// RevertibleState sets the revertible state used. Otherwise, default implementation is used.
func (o *allMoneySchemeOptions) RevertibleState(rt RevertibleState) MoneySchemeOption {
	return func(options *MoneySchemeOptions) {
		options.revertibleState = rt
	}
}
