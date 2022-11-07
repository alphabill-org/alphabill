package distributed

import (
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
)

type (
	ProposalGenerator struct {
		irChgReqBuffer map[protocol.SystemIdentifier][]*atomic_broadcast.IRChangeReqMsg
	}
)

func NewProposalGenerator() *ProposalGenerator {
	return &ProposalGenerator{
		irChgReqBuffer: make(map[protocol.SystemIdentifier][]*atomic_broadcast.IRChangeReqMsg),
	}
}

func (x *ProposalGenerator) ValidateAndBufferIRReq(req *atomic_broadcast.IRChangeReqMsg) error {
	buf, f := x.irChgReqBuffer[protocol.SystemIdentifier(req.SystemIdentifier)]
	// If there is a pending request, then compare and complain if not duplicate
	if f {
		// for now ignore
		return nil
	}
	buf = append(buf, req)
	return nil
}
