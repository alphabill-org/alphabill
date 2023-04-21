package distributed

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/distributed/storage"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	IRChangeVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *ab_consensus.IRChangeReqMsg) (*storage.InputData, error)
	}
	PartitionTimeout interface {
		GetT2Timeouts(currenRound uint64) ([]protocol.SystemIdentifier, error)
	}
	irChange struct {
		InputRecord *certificates.InputRecord
		Reason      ab_consensus.IRChangeReqMsg_CERT_REASON
		Msg         *ab_consensus.IRChangeReqMsg
	}
	IrReqBuffer struct {
		irChgReqBuffer map[protocol.SystemIdentifier]*irChange
	}
)

func NewIrReqBuffer() *IrReqBuffer {
	return &IrReqBuffer{
		irChgReqBuffer: make(map[protocol.SystemIdentifier]*irChange),
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
func (x *IrReqBuffer) Add(round uint64, irChReq *ab_consensus.IRChangeReqMsg, ver IRChangeVerifier) error {
	if irChReq == nil {
		return fmt.Errorf("ir change request is nil")
	}
	// special case, timeout cannot be requested, it can only be added to a block by the leader
	if irChReq.CertReason == ab_consensus.IRChangeReqMsg_T2_TIMEOUT {
		return fmt.Errorf("invalid ir change request, timeout can only be proposed by leader issuing a new block")
	}
	irData, err := ver.VerifyIRChangeReq(round, irChReq)
	if err != nil {
		return fmt.Errorf("ir change request verification failed, %w", err)
	}
	systemID := protocol.SystemIdentifier(irChReq.SystemIdentifier)
	// verify and extract proposed IR, NB! in this case we set the age to 0 as
	// currently no request can be received to request timeout
	newIrChReq := &irChange{InputRecord: irData.IR, Reason: irChReq.CertReason, Msg: irChReq}
	irChangeReq, found := x.irChgReqBuffer[systemID]
	if found {
		if irChangeReq.Reason != newIrChReq.Reason {
			return fmt.Errorf("error equivocating request for partition %X reason has changed", systemID.Bytes())
		}
		// compare IR's
		if compareIR(irChangeReq.InputRecord, newIrChReq.InputRecord) == true {
			// duplicate already stored
			logger.Debug("Duplicate IR change request, ignored")
			return nil
		}
		// At this point it is not possible to cast blame, so just log and ignore
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Original request for partition %X req:", systemID.Bytes()), irChangeReq)
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Equivocating request for partition %X req:", systemID.Bytes()), newIrChReq.Msg)
		return fmt.Errorf("error equivocating request for partition %X", systemID.Bytes())
	}
	// Insert first valid request received and compare the others received against it
	x.irChgReqBuffer[systemID] = newIrChReq
	return nil
}

// IsChangeInBuffer returns true if there is a request for IR change from the partition
// in the buffer
func (x *IrReqBuffer) IsChangeInBuffer(id protocol.SystemIdentifier) bool {
	_, found := x.irChgReqBuffer[id]
	return found
}

// GeneratePayload generates new proposal payload from buffered IR change requests.
func (x *IrReqBuffer) GeneratePayload(round uint64, timeouts []protocol.SystemIdentifier) *ab_consensus.Payload {
	payload := &ab_consensus.Payload{
		Requests: make([]*ab_consensus.IRChangeReqMsg, 0, len(x.irChgReqBuffer)+len(timeouts)),
	}
	// first add timeout requests
	for _, id := range timeouts {
		// if there is a request for the same partition (same id) in buffer (prefer progress to timeout) then skip
		if x.IsChangeInBuffer(id) {
			continue
		}
		logger.Debug("round %v request partition %X T2 timeout", round, id.Bytes())
		payload.Requests = append(payload.Requests, &ab_consensus.IRChangeReqMsg{
			SystemIdentifier: id.Bytes(),
			CertReason:       ab_consensus.IRChangeReqMsg_T2_TIMEOUT,
		})
	}
	for _, req := range x.irChgReqBuffer {
		payload.Requests = append(payload.Requests, req.Msg)
	}
	// clear the buffer once payload is done
	x.irChgReqBuffer = make(map[protocol.SystemIdentifier]*irChange)
	return payload
}
