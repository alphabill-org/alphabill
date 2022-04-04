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
