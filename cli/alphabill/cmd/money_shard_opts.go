package cmd

type (
	Options struct {
		shardRunFunc moneyShardRunnable
	}

	Option     func(*Options)
	allOptions struct{}
)

var (
	Opts = &allOptions{}
)

// ShardRunFunc sets the shard runnable function. Otherwise, default function will be used.
func (o *allOptions) ShardRunFunc(shardRunFunc moneyShardRunnable) Option {
	return func(options *Options) {
		options.shardRunFunc = shardRunFunc
	}
}

func convertOptsToRunnable(opts interface{}) moneyShardRunnable {
	switch v := opts.(type) {
	case moneyShardRunnable:
		return v
	case Option:
		executeOpts := Options{}
		v(&executeOpts)
		return executeOpts.shardRunFunc
	default:
		return nil
	}
}
