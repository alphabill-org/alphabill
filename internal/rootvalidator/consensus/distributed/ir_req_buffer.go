package distributed

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	IRChange struct {
		InputRecord *certificates.InputRecord
		Reason      atomic_broadcast.IRChangeReqMsg_CERT_REASON
		Msg         *atomic_broadcast.IRChangeReqMsg
	}
	IrReqBuffer struct {
		irChgReqBuffer map[protocol.SystemIdentifier]IRChange
	}
)

func NewIrReqBuffer() *IrReqBuffer {
	return &IrReqBuffer{
		irChgReqBuffer: make(map[protocol.SystemIdentifier]IRChange),
	}
}

func compareIR(a, b *certificates.InputRecord) bool {
	if bytes.Equal(a.PreviousHash, b.PreviousHash) == false {
		return false
	}
	if bytes.Equal(a.Hash, b.Hash) == false {
		return false
	}
	if bytes.Equal(a.BlockHash, b.BlockHash) == false {
		return false
	}
	if bytes.Equal(a.SummaryValue, b.SummaryValue) == false {
		return false
	}
	return true
}

// Add validates incoming IR change request and buffers valid requests. If for any reason the IR request is found not
// valid, reason is logged, error is returned and request is ignored.
func (x *IrReqBuffer) Add(newIrChReq IRChange, luc *certificates.UnicityCertificate) error {
	systemId := protocol.SystemIdentifier(newIrChReq.Msg.SystemIdentifier)
	irChangeReq, found := x.irChgReqBuffer[systemId]
	if found {
		// compare IR's
		if compareIR(irChangeReq.InputRecord, newIrChReq.InputRecord) == true {
			// duplicate already stored
			logger.Debug("Duplicate IR change request, ignored")
			return nil
		}
		// At this point it is not possible to cast blame, so just log and ignore
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Original request for partition %X req:", systemId.Bytes()), irChangeReq)
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Equivocating request for partition %X req:", systemId.Bytes()), newIrChReq.Msg)
		return fmt.Errorf("error equivocating request for partition %X", systemId.Bytes())
	}
	// Insert first valid request received and compare the others received against it
	x.irChgReqBuffer[systemId] = newIrChReq
	return nil
}

// GeneratePayload generates new proposal payload from buffered IR change requests.
func (x *IrReqBuffer) GeneratePayload() *atomic_broadcast.Payload {
	payload := &atomic_broadcast.Payload{
		Requests: make([]*atomic_broadcast.IRChangeReqMsg, len(x.irChgReqBuffer)),
	}
	i := 0
	for _, req := range x.irChgReqBuffer {
		payload.Requests[i] = req.Msg
		i++
	}
	return payload
}
