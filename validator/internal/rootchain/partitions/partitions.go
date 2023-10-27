package partitions

import (
	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
)

type (
	PartitionTrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeId string, req MsgVerification) error
	}

	PartitionConfiguration interface {
		GetInfo(id types.SystemID32) (*genesis.SystemDescriptionRecord, PartitionTrustBase, error)
	}
)
