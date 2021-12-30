package state

type (
	MoneySchemeOptions struct {
		unitsTree       UnitsTree
		revertibleState RevertibleState
	}

	MoneySchemeOption     func(*MoneySchemeOptions)
	allMoneySchemeOptions struct{}
)

var (
	MoneySchemeOpts = &allMoneySchemeOptions{}
)

// UnitsTree sets the units tree used. Otherwise, default implementation is used.
func (o *allMoneySchemeOptions) UnitsTree(ut UnitsTree) MoneySchemeOption {
	return func(options *MoneySchemeOptions) {
		options.unitsTree = ut
	}
}

// RevertibleState sets the revertible state used. Otherwise, default implementation is used.
func (o *allMoneySchemeOptions) RevertibleState(rt RevertibleState) MoneySchemeOption {
	return func(options *MoneySchemeOptions) {
		options.revertibleState = rt
	}
}
