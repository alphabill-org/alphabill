package partitions

import (
	"fmt"
	"sync"
	"sync/atomic"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

type (
	PartitionInfo struct {
		PartitionDescription *types.PartitionDescriptionRecord
		// registered nodes and their public key's
		Verifier PartitionTrustBase
	}

	TrustBase struct {
		PartitionTrustBase map[string]abcrypto.Verifier
	}

	ConfigurationStore interface {
		GetConfiguration(round uint64) (*genesis.RootGenesis, uint64, error)
	}

	PartitionStore struct {
		mu           sync.RWMutex
		cfgStore     ConfigurationStore
		cfgVersion   atomic.Uint64
		// cached configuration of partitions from the latest
		// GetInfo call, usually does not change
		partitions   map[types.SystemID]*PartitionInfo
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
	ver, found := v.PartitionTrustBase[nodeId]
	if !found {
		return fmt.Errorf("node %s is not part of partition trustbase", nodeId)
	}
	return req.IsValid(ver)
}

func NewPartitionStore(cfgStore ConfigurationStore) (*PartitionStore, error) {
	if cfgStore == nil {
		return nil, fmt.Errorf("configuration storage must be initialized")
	}
	return &PartitionStore{cfgStore: cfgStore}, nil
}

func (ps *PartitionStore) GetInfo(id types.SystemID, round uint64) (*types.PartitionDescriptionRecord, PartitionTrustBase, error) {
	if err := ps.loadConfig(round); err != nil {
		return nil, nil, fmt.Errorf("loading new configuration: %w", err)
	}

	ps.mu.RLock()
	defer ps.mu.RUnlock()

	info, f := ps.partitions[id]
	if !f {
		return nil, nil, fmt.Errorf("unknown partition identifier %s", id)
	}
	return info.PartitionDescription, info.Verifier, nil
}

/*
loadConfig loads the root chain configuration for round "round" and
updates the partition info cache.
*/
func (ps *PartitionStore) loadConfig(round uint64) error {
	cfg, version, err := ps.cfgStore.GetConfiguration(round)
	if err != nil {
		return fmt.Errorf("loading from configuration store: %w", err)
	}
	if ps.cfgVersion.Load() == version {
		// Correct configuration already loaded
		return nil
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	// double-check to see if someone else already loaded it while we acquired lock
	if ps.cfgVersion.Load() == version {
		return nil
	}

	partitions := make(map[types.SystemID]*PartitionInfo)
	for _, partition := range cfg.Partitions {
		trustBase := make(map[string]abcrypto.Verifier)
		for _, node := range partition.Nodes {
			ver, err := abcrypto.NewVerifierSecp256k1(node.SigningPublicKey)
			if err != nil {
				return fmt.Errorf("creating verifier for the node %q: %w", node.NodeIdentifier, err)
			}
			trustBase[node.NodeIdentifier] = ver
		}
		partitions[partition.PartitionDescription.SystemIdentifier] = &PartitionInfo{
			PartitionDescription: partition.PartitionDescription,
			Verifier:          NewPartitionTrustBase(trustBase),
		}
	}

	ps.partitions = partitions
	ps.cfgVersion.Store(version)

	return nil
}
