package rpc

type (
	Options struct {
		converter TransactionOrderConverter
	}

	Option     func(*Options)
	allOptions struct{}
)

var (
	Opts = &allOptions{}
)

func (a *allOptions) TransactionOrderConverter(c TransactionOrderConverter) Option {
	return func(options *Options) {
		options.converter = c
	}
}
