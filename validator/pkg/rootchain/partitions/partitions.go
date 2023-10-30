package partitions

import (
	"github.com/alphabill-org/alphabill/api/sdr"
	"github.com/alphabill-org/alphabill/api/types"
)

type (
	PartitionTrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeId string, req MsgVerification) error
	}

	PartitionConfiguration interface {
		GetInfo(id types.SystemID32) (*sdr.SystemDescriptionRecord, PartitionTrustBase, error)
	}
)
