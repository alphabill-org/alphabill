package abdrc

import (
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

type (
	IRChangeVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *abtypes.IRChangeReq) (*storage.InputData, error)
	}
	PartitionTimeout interface {
		GetT2Timeouts(currenRound uint64) ([]protocol.SystemIdentifier, error)
	}
	irChange struct {
		InputRecord *types.InputRecord
		Reason      abtypes.IRChangeReason
		Req         *abtypes.IRChangeReq
	}
	IrReqBuffer struct {
		irChgReqBuffer map[protocol.SystemIdentifier]*irChange
		log            *slog.Logger
	}
)

func NewIrReqBuffer(log *slog.Logger) *IrReqBuffer {
	return &IrReqBuffer{
		irChgReqBuffer: make(map[protocol.SystemIdentifier]*irChange),
		log:            log,
	}
}

// Add validates incoming IR change request and buffers valid requests. If for any reason the IR request is found not
// valid, reason is logged, error is returned and request is ignored.
func (x *IrReqBuffer) Add(round uint64, irChReq *abtypes.IRChangeReq, ver IRChangeVerifier) error {
	if irChReq == nil {
		return fmt.Errorf("ir change request is nil")
	}
	// special case, timeout cannot be requested, it can only be added to a block by the leader
	if irChReq.CertReason == abtypes.T2Timeout {
		return fmt.Errorf("invalid ir change request, timeout can only be proposed by leader issuing a new block")
	}
	irData, err := ver.VerifyIRChangeReq(round, irChReq)
	if err != nil {
		return fmt.Errorf("ir change request verification failed, %w", err)
	}
	systemID := protocol.SystemIdentifier(irChReq.SystemIdentifier)
	// verify and extract proposed IR, NB! in this case we set the age to 0 as
	// currently no request can be received to request timeout
	newIrChReq := &irChange{InputRecord: irData.IR, Reason: irChReq.CertReason, Req: irChReq}
	if irChangeReq, found := x.irChgReqBuffer[systemID]; found {
		if irChangeReq.Reason != newIrChReq.Reason {
			return fmt.Errorf("equivocating request for partition %X, reason has changed", systemID.Bytes())
		}
		if irChangeReq.InputRecord.Equal(newIrChReq.InputRecord) {
			// duplicate already stored
			x.log.Debug("Duplicate IR change request, ignored", logger.Round(round))
			return nil
		}
		// At this point it is not possible to cast blame, so just log and ignore
		x.log.Debug(fmt.Sprintf("equivocating request for partition %X", systemID.Bytes()), logger.Round(round), logger.Data(newIrChReq.Req), logger.Data(irChangeReq))
		return fmt.Errorf("equivocating request for partition %X", systemID.Bytes())
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
func (x *IrReqBuffer) GeneratePayload(round uint64, timeouts []protocol.SystemIdentifier) *abtypes.Payload {
	payload := &abtypes.Payload{
		Requests: make([]*abtypes.IRChangeReq, 0, len(x.irChgReqBuffer)+len(timeouts)),
	}
	// first add timeout requests
	for _, id := range timeouts {
		// if there is a request for the same partition (same id) in buffer (prefer progress to timeout) then skip
		if x.IsChangeInBuffer(id) {
			continue
		}
		x.log.Debug(fmt.Sprintf("partition %X request T2 timeout", id.Bytes()), logger.Round(round))
		payload.Requests = append(payload.Requests, &abtypes.IRChangeReq{
			SystemIdentifier: id.Bytes(),
			CertReason:       abtypes.T2Timeout,
		})
	}
	for _, req := range x.irChgReqBuffer {
		payload.Requests = append(payload.Requests, req.Req)
	}
	// clear the buffer once payload is done
	x.irChgReqBuffer = make(map[protocol.SystemIdentifier]*irChange)
	return payload
}
