package partitions

import (
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

type (
	PartitionTrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeId string, req MsgVerification) error
	}

	PartitionConfiguration interface {
		GetInfo(id protocol.SystemIdentifier) (*genesis.SystemDescriptionRecord, PartitionTrustBase, error)
	}
)
