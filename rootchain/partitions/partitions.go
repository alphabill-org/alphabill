package partitions

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

type (
	PartitionTrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeId string, req MsgVerification) error
	}

	PartitionConfiguration interface {
		Reset(curRound func() uint64) error
		GetInfo(id types.SystemID, round uint64) (*types.SystemDescriptionRecord, PartitionTrustBase, error)
		AddConfiguration(round uint64, cfg *genesis.RootGenesis) error
	}
)
