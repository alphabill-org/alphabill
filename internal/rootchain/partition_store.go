package rootchain

import (
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

// PartitionStore stores partition related information. key of the map is system identifier.
type PartitionStore map[p.SystemIdentifier]*genesis.PartitionRecord

// NewPartitionStore creates a new partition store with given partitions.
func NewPartitionStore(partitions []*genesis.PartitionRecord) *PartitionStore {
	s := PartitionStore(make(map[p.SystemIdentifier]*genesis.PartitionRecord))
	for _, partition := range partitions {
		identifier := p.SystemIdentifier(partition.GetSystemIdentifier())
		s[identifier] = partition
	}
	return &s
}

// Size returns the number of partition in the partition store.
func (ps *PartitionStore) Size() int {
	return len(*ps)
}

// NodeCount returns the number of nodes in the given partition.
// If partition is not in the partitionStore then 0 is returned.
func (ps *PartitionStore) NodeCount(id p.SystemIdentifier) int {
	p, f := (*ps)[id]
	if !f {
		return 0
	}
	return len(p.Validators)
}

// Get returns the partition or nil if not found.
func (ps *PartitionStore) Get(id p.SystemIdentifier) *genesis.PartitionRecord {
	return (*ps)[id]
}
