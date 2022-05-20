package cmd

type (
	Options struct {
		nodeRunFunc moneyNodeRunnable
	}

	Option     func(*Options)
	allOptions struct{}
)

var (
	Opts = &allOptions{}
)

// NodeRunFunc sets the node runnable function. Otherwise, default function will be used.
func (o *allOptions) NodeRunFunc(shardRunFunc moneyNodeRunnable) Option {
	return func(options *Options) {
		options.nodeRunFunc = shardRunFunc
	}
}

func convertOptsToRunnable(opts interface{}) moneyNodeRunnable {
	switch v := opts.(type) {
	case moneyNodeRunnable:
		return v
	case Option:
		executeOpts := Options{}
		v(&executeOpts)
		return executeOpts.nodeRunFunc
	default:
		return nil
	}
}
