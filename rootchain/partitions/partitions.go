package partitions

import (
	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
)

type (
	PartitionTrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeId string, f func(v abcrypto.Verifier) error) error
	}

	PartitionConfiguration interface {
		Reset(curRound func() uint64) error
		GetInfo(id types.PartitionID, round uint64) (*types.PartitionDescriptionRecord, PartitionTrustBase, error)
	}
)
