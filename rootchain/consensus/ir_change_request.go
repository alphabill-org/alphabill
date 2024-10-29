package consensus

import (
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

const (
	Quorum CertReqReason = iota
	QuorumNotPossible
)

type (
	CertReqReason uint8

	IRChangeRequest struct {
		Partition types.PartitionID
		Shard     types.ShardID
		Reason    CertReqReason
		Requests  []*certification.BlockCertificationRequest
	}
)
