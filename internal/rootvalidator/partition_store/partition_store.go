package partition_store

import (
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill/internal/crypto"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

type PartitionInfo struct {
	// partition system description record,
	SystemDescription *genesis.SystemDescriptionRecord
	// registered nodes and their public key's
	TrustBase map[string]crypto.Verifier
}

// PartitionStore stores partition related information. key of the map is system identifier.
type PartitionStore struct {
	mu         sync.Mutex
	partitions map[p.SystemIdentifier]*PartitionInfo
}

// GetQuorum calculates and returns minimum number of nodes required for a quorum
func (x PartitionInfo) GetQuorum() uint64 {
	// Partition quorum is currently set to 50%, meaning at least
	// +1 to round up and avoid using floats
	return uint64(len(x.TrustBase)/2) + 1
}

// NewPartitionStoreFromGenesis creates a new partition store from root genesis.
func NewPartitionStoreFromGenesis(partitions []*genesis.GenesisPartitionRecord) (*PartitionStore, error) {
	parts := make(map[p.SystemIdentifier]*PartitionInfo)
	for _, partition := range partitions {
		identifier := p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)
		trustBase := make(map[string]crypto.Verifier)
		for _, node := range partition.Nodes {
			ver, err := crypto.NewVerifierSecp256k1(node.SigningPublicKey)
			if err != nil {
				return nil, err
			}
			trustBase[node.NodeIdentifier] = ver
		}
		parts[identifier] = &PartitionInfo{SystemDescription: partition.SystemDescriptionRecord,
			TrustBase: trustBase}
	}
	return &PartitionStore{partitions: parts}, nil
}

func (ps *PartitionStore) Info(id p.SystemIdentifier) (PartitionInfo, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	info, f := ps.partitions[id]
	if !f {
		return PartitionInfo{}, fmt.Errorf("unknown system identifier %X", id)
	}
	return *info, nil
}
