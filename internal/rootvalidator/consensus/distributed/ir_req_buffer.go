package distributed

import (
	"bytes"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/request_store"
)

type (
	IRChange struct {
		InputRecord *certificates.InputRecord
		Reason      atomic_broadcast.IRChangeReqMsg_CERT_REASON
		Msg         *atomic_broadcast.IRChangeReqMsg
	}
	IrReqBuffer struct {
		irChgReqBuffer map[protocol.SystemIdentifier]*IRChange
	}
)

func NewIrReqBuffer() *IrReqBuffer {
	return &IrReqBuffer{
		irChgReqBuffer: make(map[protocol.SystemIdentifier]*IRChange),
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
func (x *IrReqBuffer) Add(req *atomic_broadcast.IRChangeReqMsg, luc *certificates.UnicityCertificate, nofNodes int) error {
	systemId := protocol.SystemIdentifier(req.SystemIdentifier)

	// verify timeout, timeout is special no proof payload, current round number
	if req.CertReason == atomic_broadcast.IRChangeReqMsg_T2_TIMEOUT {
		// todo: verify timeout - refactor and make method for IRChangeReqMsg to verify proof
		x.irChgReqBuffer[systemId] = &IRChange{InputRecord: luc.InputRecord, Reason: req.CertReason, Msg: req}
		return nil
	}

	irChangeReq, found := x.irChgReqBuffer[systemId]
	// If there is a pending request, then compare and complain if not duplicate
	requestStore := request_store.NewCertificationRequestStore()
	// Verify proof
	for _, r := range req.Requests {
		// Verify system identifier matches requests
		if !bytes.Equal(systemId.Bytes(), req.SystemIdentifier) {
			return fmt.Errorf("IR change request system id %X, does not match proof system id %X", systemId.Bytes(), req.SystemIdentifier)
		}
		// check request against last state, filter out:
		// 1. requests that extend invalid or old state
		// 2. requests that are stale
		if err := consensus.CheckBlockCertificationRequest(r, luc); err != nil {
			return fmt.Errorf("IR change request proof error: %w", err)
		}
		if err := requestStore.Add(r); err != nil {
			return fmt.Errorf("IR change request proof error: %w", err)
		}
	}
	ir, _ := requestStore.IsConsensusReceived(systemId, nofNodes)
	if ir == nil {
		return fmt.Errorf("invalid IR change request no consensus reached")
	}
	if found {
		// compare IR's
		if compareIR(irChangeReq.InputRecord, ir) == true {
			logger.Debug("Duplicate IR change request, ignored")
			return nil
		}
		// At this point it is not possible to cast blame, so just log and ignore
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Original request for partition %X req:", systemId.Bytes()), irChangeReq)
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Equivocating request for partition %X req:", systemId.Bytes()), req)
		return fmt.Errorf("error equivocating request for partition %X", systemId.Bytes())
	}
	// Insert first valid request received and compare the others received against it
	x.irChgReqBuffer[systemId] = &IRChange{InputRecord: ir, Reason: req.CertReason, Msg: req}
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
