package partitions

import (
	"fmt"
	"maps"
	"slices"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
)

type (
	TrustBase struct {
		PartitionTrustBase map[string]abcrypto.Verifier
	}
)

func NewPartitionTrustBase(tb map[string]abcrypto.Verifier) *TrustBase {
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

func (v *TrustBase) Verify(nodeId string, f func(v abcrypto.Verifier) error) error {
	ver, found := v.PartitionTrustBase[nodeId]
	if !found {
		return fmt.Errorf("node %s is not part of partition trustbase", nodeId)
	}
	return f(ver)
}
