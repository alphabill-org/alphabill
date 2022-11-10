package partition_store

import (
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

type PartitionInfo struct {
	// partition system description record,
	SystemDescription genesis.SystemDescriptionRecord
	// registered nodes and their public key's
	TrustBase map[string]crypto.Verifier
}

// PartitionStore stores partition related information. key of the map is system identifier.
type PartitionStore struct {
	mu         sync.Mutex
	partitions map[p.SystemIdentifier]*PartitionInfo
}

func NewEmptyPartitionStore() *PartitionStore {
	return &PartitionStore{partitions: make(map[p.SystemIdentifier]*PartitionInfo)}
}

// NewPartitionStore creates a new partition store with given partitions.
func NewPartitionStore(partitions []*genesis.PartitionRecord) (*PartitionStore, error) {
	parts := make(map[p.SystemIdentifier]*PartitionInfo)
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
		parts[identifier] = &PartitionInfo{SystemDescription: *partition.SystemDescriptionRecord,
			TrustBase: trustBase}
	}
	return &PartitionStore{partitions: parts}, nil
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
		parts[identifier] = &PartitionInfo{SystemDescription: *partition.SystemDescriptionRecord,
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
		ps.partitions = make(map[p.SystemIdentifier]*PartitionInfo)
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
	ps.partitions[sysIdent] = &PartitionInfo{SystemDescription: *partition.SystemDescriptionRecord,
		TrustBase: trustBase}
	return nil
}

func (ps *PartitionStore) GetSystemDescriptions() []*genesis.SystemDescriptionRecord {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	descriptions := make([]*genesis.SystemDescriptionRecord, len(ps.partitions))
	i := 0
	for _, info := range ps.partitions {
		descriptions[i] = &info.SystemDescription
		i++
	}
	return descriptions
}

// Size returns the number of partition in the partition store.
func (ps *PartitionStore) Size() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return len(ps.partitions)
}

func (ps *PartitionStore) GetInfo(id p.SystemIdentifier) (PartitionInfo, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	info, f := ps.partitions[id]
	if !f {
		return PartitionInfo{}, fmt.Errorf("unknown system identifier %X", id)
	}
	return *info, nil
}
