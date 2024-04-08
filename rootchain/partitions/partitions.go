package partitions

import (
	"github.com/alphabill-org/alphabill/types"
)

type (
	PartitionTrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeId string, req MsgVerification) error
	}

	PartitionConfiguration interface {
		GetInfo(id types.SystemID) (*types.SystemDescriptionRecord, PartitionTrustBase, error)
	}
)
