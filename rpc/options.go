package rpc

type (
	Options struct {
		maxGetBlocksBatchSize uint64
	}

	Option func(*Options)
)

func WithMaxGetBlocksBatchSize(maxGetBlocksBatchSize uint64) Option {
	return func(c *Options) {
		c.maxGetBlocksBatchSize = maxGetBlocksBatchSize
	}
}
