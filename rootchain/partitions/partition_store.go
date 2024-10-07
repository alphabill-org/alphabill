package partitions

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"sync"

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

	GenesisStore interface {
		AddConfiguration(round uint64, cfg *genesis.RootGenesis) error
		PartitionRecords(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error)
	}

	PartitionStore struct {
		mu sync.Mutex
		// currently active partitions configuration
		partitions map[types.SystemID]*PartitionInfo

		next     uint64        // round number when next cfg must be activated
		curRound func() uint64 // returns current round number.
		cfgStore GenesisStore
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

func (v *TrustBase) NodeIDs() []string {
	return slices.Collect(maps.Keys(v.PartitionTrustBase))
}

func (v *TrustBase) Verify(nodeId string, req MsgVerification) error {
	ver, found := v.PartitionTrustBase[nodeId]
	if !found {
		return fmt.Errorf("node %s is not part of partition trustbase", nodeId)
	}
	return req.IsValid(ver)
}

/*
NB! The PartitionStore returned is NOT ready for use, the Reset method
must be called before any other method is called!
*/
func NewPartitionStore(cfgStore GenesisStore) (*PartitionStore, error) {
	if cfgStore == nil {
		return nil, fmt.Errorf("genesis storage must be initialized")
	}
	return &PartitionStore{cfgStore: cfgStore, next: math.MaxUint64}, nil
}

/*
Reset sets the source of truth about current round number and loads the configuration
of the current round (ie what is returned by the curRound callback). IOW this method
must be called once system is in the state where it knows current round.
*/
func (ps *PartitionStore) Reset(curRound func() uint64) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.curRound = curRound
	return ps.loadConfig(curRound())
}

/*
It is expected that "round" only increases, to jump back in history Reset has to be called.
*/
func (ps *PartitionStore) GetInfo(id types.SystemID, round uint64) (*types.PartitionDescriptionRecord, PartitionTrustBase, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.next <= round {
		if err := ps.loadConfig(round); err != nil {
			return nil, nil, fmt.Errorf("switching to new config: %w", err)
		}
	}

	info, f := ps.partitions[id]
	if !f {
		return nil, nil, fmt.Errorf("unknown partition identifier %s", id)
	}
	return info.PartitionDescription, info.Verifier, nil
}

/*
AddConfiguration registers new configuration starting from round "round". This method
is meant to be used to add configurations for future rounds, not to add historic data.
*/
func (ps *PartitionStore) AddConfiguration(round uint64, cfg *genesis.RootGenesis) error {
	if cr := ps.curRound(); cr >= round {
		return fmt.Errorf("can't add config taking effect on round %d as current round is already %d", round, cr)
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	if err := ps.cfgStore.AddConfiguration(round, cfg); err != nil {
		return fmt.Errorf("storing configurations for round %d: %w", round, err)
	}
	if ps.next > round {
		ps.next = round
	}

	return nil
}

/*
loadConfig sets the current config for the one of round "round".

NB! method assumes that the caller holds the mutex!
*/
func (ps *PartitionStore) loadConfig(round uint64) error {
	cfg, next, err := ps.cfgStore.PartitionRecords(round)
	if err != nil {
		return fmt.Errorf("loading genesis for the round %d: %w", round, err)
	}

	parts := make(map[types.SystemID]*PartitionInfo)
	for _, partition := range cfg {
		trustBase := make(map[string]abcrypto.Verifier)
		for _, node := range partition.Nodes {
			ver, err := abcrypto.NewVerifierSecp256k1(node.SigningPublicKey)
			if err != nil {
				return fmt.Errorf("creating verifier for the node %q: %w", node.NodeIdentifier, err)
			}
			trustBase[node.NodeIdentifier] = ver
		}
		parts[partition.PartitionDescription.SystemIdentifier] = &PartitionInfo{
			PartitionDescription: partition.PartitionDescription,
			Verifier:             NewPartitionTrustBase(trustBase),
		}
	}

	ps.partitions = parts
	ps.next = next
	return nil
}
