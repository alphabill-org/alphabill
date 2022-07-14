package rootchain

import (
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

// partitionStore stores partition related information. key of the map is system identifier.
type partitionStore map[string]*genesis.PartitionRecord

// newPartitionStore creates a new partition store with given partitions.
func newPartitionStore(partitions []*genesis.PartitionRecord) *partitionStore {
	s := partitionStore(make(map[string]*genesis.PartitionRecord))
	for _, partition := range partitions {
		identifier := partition.GetSystemIdentifierString()
		s[identifier] = partition
	}
	return &s
}

// size returns the number of partition in the partition store.
func (ps *partitionStore) size() int {
	return len(*ps)
}

// nodeCount returns the number of nodes in the given partition.
// If partition is not in the partitionStore then 0 is returned.
func (ps *partitionStore) nodeCount(id string) int {
	p, f := (*ps)[id]
	if !f {
		return 0
	}
	return len(p.Validators)
}

// get returns the partition or nil if not found.
func (ps *partitionStore) get(id string) *genesis.PartitionRecord {
	return (*ps)[id]
}
