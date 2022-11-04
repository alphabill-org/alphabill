package partition_store

import (
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

type partitionInfo struct {
	// partition system description record,
	SystemDescription *genesis.SystemDescriptionRecord
	// registered nodes and their public key's
	TrustBase map[string]crypto.Verifier
}

// PartitionStore stores partition related information. key of the map is system identifier.
type PartitionStore struct {
	mu         sync.Mutex
	partitions map[p.SystemIdentifier]*partitionInfo
}

func NewEmptyPartitionStore() *PartitionStore {
	return &PartitionStore{partitions: make(map[p.SystemIdentifier]*partitionInfo)}
}

// NewPartitionStore creates a new partition store with given partitions.
func NewPartitionStore(partitions []*genesis.PartitionRecord) (*PartitionStore, error) {
	parts := make(map[p.SystemIdentifier]*partitionInfo)
	for _, partition := range partitions {
		identifier := p.SystemIdentifier(partition.GetSystemIdentifier())
		trustBase := make(map[string]crypto.Verifier)
		for _, node := range partition.Validators {
			if node == nil {
				return nil, errors.New("invalid partition record, node is nil")
			}
			ver, err := crypto.NewVerifierSecp256k1(node.SigningPublicKey)
			if err != nil {
				return nil, err
			}
			trustBase[node.NodeIdentifier] = ver
		}
		parts[identifier] = &partitionInfo{SystemDescription: partition.SystemDescriptionRecord,
			TrustBase: trustBase}
	}
	return &PartitionStore{partitions: parts}, nil
}

// NewPartitionStoreFromGenesis creates a new partition store from root genesis.
func NewPartitionStoreFromGenesis(partitions []*genesis.GenesisPartitionRecord) (*PartitionStore, error) {
	parts := make(map[p.SystemIdentifier]*partitionInfo)
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
		parts[identifier] = &partitionInfo{SystemDescription: partition.SystemDescriptionRecord,
			TrustBase: trustBase}
	}
	return &PartitionStore{partitions: parts}, nil
}

func (ps *PartitionStore) AddPartition(partition *genesis.PartitionRecord) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	sysIdent := p.SystemIdentifier(partition.SystemDescriptionRecord.SystemIdentifier)
	// init on first add
	if ps.partitions == nil {
		ps.partitions = make(map[p.SystemIdentifier]*partitionInfo)
	}
	_, f := ps.partitions[sysIdent]
	if f {
		return fmt.Errorf("partition with system ident %X already exists", sysIdent)
	}
	trustBase := make(map[string]crypto.Verifier)
	for _, node := range partition.Validators {
		ver, err := crypto.NewVerifierSecp256k1(node.SigningPublicKey)
		if err != nil {
			return err
		}
		trustBase[node.NodeIdentifier] = ver
	}
	ps.partitions[sysIdent] = &partitionInfo{SystemDescription: partition.SystemDescriptionRecord,
		TrustBase: trustBase}
	return nil
}

// Size returns the number of partition in the partition store.
func (ps *PartitionStore) Size() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return len(ps.partitions)
}

// NodeCount returns the number of nodes in the given partition.
// If partition is not in the partitionStore then 0 is returned.
func (ps *PartitionStore) NodeCount(id p.SystemIdentifier) int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, f := ps.partitions[id]
	if !f {
		return 0
	}
	return len(p.TrustBase)
}

func (ps *PartitionStore) GetSystemDescription(id p.SystemIdentifier) (*genesis.SystemDescriptionRecord, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, f := ps.partitions[id]
	if !f {
		return nil, fmt.Errorf("unknown system identifier %X", id)
	}
	return p.SystemDescription, nil
}

// GetNodes returns all registered partition nodes
func (ps *PartitionStore) GetNodes(id p.SystemIdentifier) ([]string, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, f := ps.partitions[id]
	if !f {
		return nil, fmt.Errorf("unknown system identifier %X", id)
	}
	nodes := make([]string, len(p.TrustBase))
	i := 0
	for k := range p.TrustBase {
		nodes[i] = k
		i++
	}

	return nodes, nil
}

// GetTrustBase returns partition map of registered nodes and their keys
func (ps *PartitionStore) GetTrustBase(id p.SystemIdentifier) (map[string]crypto.Verifier, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, f := ps.partitions[id]
	if !f {
		return nil, fmt.Errorf("unknown system identifier %X", id)
	}
	return p.TrustBase, nil
}
