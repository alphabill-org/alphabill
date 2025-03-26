package rpc

import (
	"golang.org/x/time/rate"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/partition"
)

type (
	StateAPIOptions struct {
		withGetUnits             bool
		pdr                      *types.PartitionDescriptionRecord
		ownerIndex               partition.IndexReader
		getUnitsByOwnerIDLimiter *rate.Limiter
	}

	StateAPIOption func(*StateAPIOptions)
)

func WithGetUnits(withGetUnits bool) StateAPIOption {
	return func(c *StateAPIOptions) {
		c.withGetUnits = withGetUnits
	}
}

func WithPDR(pdr *types.PartitionDescriptionRecord) StateAPIOption {
	return func(c *StateAPIOptions) {
		c.pdr = pdr
	}
}

func WithOwnerIndex(ownerIndex partition.IndexReader) StateAPIOption {
	return func(c *StateAPIOptions) {
		c.ownerIndex = ownerIndex
	}
}

func WithGetUnitsByOwnerIDRateLimit(rateLimit int) StateAPIOption {
	return func(c *StateAPIOptions) {
		if rateLimit == 0 {
			c.getUnitsByOwnerIDLimiter = rate.NewLimiter(rate.Inf, 0)
		} else {
			c.getUnitsByOwnerIDLimiter = rate.NewLimiter(rate.Limit(rateLimit), rateLimit)
		}
	}
}

func defaultStateAPIOptions() *StateAPIOptions {
	return &StateAPIOptions{
		withGetUnits:             false,
		pdr:                      nil,
		ownerIndex:               nil,
		getUnitsByOwnerIDLimiter: rate.NewLimiter(rate.Inf, 0),
	}
}
