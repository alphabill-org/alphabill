package consensus

import (
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
)

type CertReqReason uint8

const (
	Quorum CertReqReason = iota
	QuorumNotPossible
)

type (
	IRChangeRequest struct {
		SystemIdentifier protocol.SystemIdentifier
		Reason           CertReqReason
		IR               *certificates.InputRecord
		Requests         []*certification.BlockCertificationRequest
	}
)
