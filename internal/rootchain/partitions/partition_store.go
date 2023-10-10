package partitions

import (
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	PartitionInfo struct {
		// partition system description record,
		SystemDescription *genesis.SystemDescriptionRecord
		// registered nodes and their public key's
		Verifier PartitionTrustBase
	}

	TrustBase struct {
		PartitionTrustBase map[string]crypto.Verifier
	}

	// PartitionStore stores partition related information. key of the map is system identifier.
	PartitionStore struct {
		mu         sync.Mutex
		partitions map[types.SystemID32]*PartitionInfo
	}

	MsgVerification interface {
		IsValid(v crypto.Verifier) error
	}
)

func NewPartitionTrustBase(tb map[string]crypto.Verifier) PartitionTrustBase {
	return &TrustBase{PartitionTrustBase: tb}
}

// GetQuorum calculates and returns minimum number of nodes required for a quorum
func (v *TrustBase) GetQuorum() uint64 {
	// Partition quorum is currently set to 50%, meaning at least
	// +1 to round up and avoid using floats
	return uint64(len(v.PartitionTrustBase)/2) + 1
}

// GetTotalNodes returns total number of registered validator nodes
func (v *TrustBase) GetTotalNodes() uint64 {
	return uint64(len(v.PartitionTrustBase))
}

func (v *TrustBase) Verify(nodeId string, req MsgVerification) error {
	// find node verifier
	ver, found := v.PartitionTrustBase[nodeId]
	if !found {
		return fmt.Errorf("verification failed, unknown node id %v", nodeId)
	}
	return req.IsValid(ver)
}

// NewPartitionStoreFromGenesis creates a new partition store from root genesis.
func NewPartitionStoreFromGenesis(partitions []*genesis.GenesisPartitionRecord) (*PartitionStore, error) {
	parts := make(map[types.SystemID32]*PartitionInfo)
	for _, partition := range partitions {
		identifier := types.SystemID(partition.SystemDescriptionRecord.SystemIdentifier)
		trustBase := make(map[string]crypto.Verifier)
		for _, node := range partition.Nodes {
			ver, err := crypto.NewVerifierSecp256k1(node.SigningPublicKey)
			if err != nil {
				return nil, err
			}
			trustBase[node.NodeIdentifier] = ver
		}
		parts[identifier.ToSystemID32()] = &PartitionInfo{SystemDescription: partition.SystemDescriptionRecord,
			Verifier: NewPartitionTrustBase(trustBase)}
	}
	return &PartitionStore{partitions: parts}, nil
}

func (ps *PartitionStore) GetInfo(id types.SystemID32) (*genesis.SystemDescriptionRecord, PartitionTrustBase, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	info, f := ps.partitions[id]
	if !f {
		return nil, nil, fmt.Errorf("unknown system identifier %X", id)
	}
	return info.SystemDescription, info.Verifier, nil
}
