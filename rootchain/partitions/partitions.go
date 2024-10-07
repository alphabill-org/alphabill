package partitions

import (
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	PartitionTrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeId string, req MsgVerification) error
	}

	ShardTrustBase interface {
		// NodeIDs returns list of validator node IDs
		NodeIDs() []string
	}

	PartitionConfiguration interface {
		GetInfo(id types.SystemID, round uint64) (*types.PartitionDescriptionRecord, PartitionTrustBase, error)
	}
)
