package cmd

import "context"

type (
	Options struct {
		shardNodeRunFn nodeRunnable
	}

	Option     func(*Options)
	allOptions struct{}

	// nodeRunnable is the function that is run after configuration is loaded.
	nodeRunnable func(ctx context.Context, flags *ShardNodeRunFlags) error
)

var (
	Opts = &allOptions{}
)

// NodeRunFunc sets the node runnable function. Otherwise, default function will be used.
func (o *allOptions) ShardNodeRunFn(shardNodeRunFn nodeRunnable) Option {
	return func(options *Options) {
		options.shardNodeRunFn = shardNodeRunFn
	}
}

func convertOptsToRunnable(opts interface{}) nodeRunnable {
	switch v := opts.(type) {
	case nodeRunnable:
		return v
	case Option:
		executeOpts := Options{}
		v(&executeOpts)
		return executeOpts.shardNodeRunFn
	case []interface{}:
		for _, op := range v {
			if f := convertOptsToRunnable(op); f != nil {
				return f
			}
		}
		return nil
	default:
		return nil
	}
}
