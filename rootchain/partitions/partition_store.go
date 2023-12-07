package partitions

import (
	"fmt"
	"sync"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/types"
)

type (
	PartitionInfo struct {
		// partition system description record,
		SystemDescription *genesis.SystemDescriptionRecord
		// registered nodes and their public key's
		Verifier PartitionTrustBase
	}

	TrustBase struct {
		PartitionTrustBase map[string]abcrypto.Verifier
	}

	// PartitionStore stores partition related information. key of the map is system identifier.
	PartitionStore struct {
		mu         sync.Mutex
		partitions map[types.SystemID32]*PartitionInfo
	}

	MsgVerification interface {
		IsValid(v abcrypto.Verifier) error
	}
)

func NewPartitionTrustBase(tb map[string]abcrypto.Verifier) PartitionTrustBase {
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
		sysID, err := partition.SystemDescriptionRecord.SystemIdentifier.Id32()
		if err != nil {
			return nil, err
		}
		trustBase := make(map[string]abcrypto.Verifier)
		for _, node := range partition.Nodes {
			ver, err := abcrypto.NewVerifierSecp256k1(node.SigningPublicKey)
			if err != nil {
				return nil, err
			}
			trustBase[node.NodeIdentifier] = ver
		}
		parts[sysID] = &PartitionInfo{SystemDescription: partition.SystemDescriptionRecord,
			Verifier: NewPartitionTrustBase(trustBase)}
	}
	return &PartitionStore{partitions: parts}, nil
}

func (ps *PartitionStore) GetInfo(id types.SystemID32) (*genesis.SystemDescriptionRecord, PartitionTrustBase, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	info, f := ps.partitions[id]
	if !f {
		return nil, nil, fmt.Errorf("unknown system identifier %s", id)
	}
	return info.SystemDescription, info.Verifier, nil
}
