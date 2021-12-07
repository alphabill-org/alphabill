package cmd

type (
	Options struct {
		shardRunFunc shardRunnable
	}

	Option     func(*Options)
	allOptions struct{}
)

var (
	Opts = &allOptions{}
)

// ShardRunFunc sets the shard runnable function. Otherwise, default function will be used.
func (o *allOptions) ShardRunFunc(shardRunFunc shardRunnable) Option {
	return func(options *Options) {
		options.shardRunFunc = shardRunFunc
	}
}
