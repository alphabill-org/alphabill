package rpc

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/partition"
)

type (
	StateAPIOptions struct {
		withGetUnits bool
		pdr          *types.PartitionDescriptionRecord
		ownerIndex   partition.IndexReader
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

func defaultStateAPIOptions() *StateAPIOptions {
	return &StateAPIOptions{
		withGetUnits: false,
		pdr:          nil,
		ownerIndex:   nil,
	}
}
