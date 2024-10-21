package consensus

import (
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
	IRChangeVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error)
	}
	PartitionTimeout interface {
		GetT2Timeouts(currenRound uint64) ([]types.SystemID, error)
	}
	irChange struct {
		InputRecord *types.InputRecord
		Reason      drctypes.IRChangeReason
		Req         *drctypes.IRChangeReq
	}
	IrReqBuffer struct {
		irChgReqBuffer map[types.SystemID]*irChange
		log            *slog.Logger
	}

	InProgressFn func(id32 types.SystemID) *types.InputRecord
)

func NewIrReqBuffer(log *slog.Logger) *IrReqBuffer {
	return &IrReqBuffer{
		irChgReqBuffer: make(map[types.SystemID]*irChange),
		log:            log,
	}
}

// Add validates incoming IR change request and buffers valid requests. If for any reason the IR request is found not
// valid, reason is logged, error is returned and request is ignored.
func (x *IrReqBuffer) Add(round uint64, irChReq *drctypes.IRChangeReq, ver IRChangeVerifier) error {
	if irChReq == nil {
		return fmt.Errorf("ir change request is nil")
	}
	// special case, timeout cannot be requested, it can only be added to a block by the leader
	if irChReq.CertReason == drctypes.T2Timeout {
		return fmt.Errorf("invalid ir change request, timeout can only be proposed by leader issuing a new block")
	}
	irData, err := ver.VerifyIRChangeReq(round, irChReq)
	if err != nil {
		return fmt.Errorf("ir change request verification failed, %w", err)
	}
	systemID := irChReq.Partition
	// verify and extract proposed IR, NB! in this case we set the age to 0 as
	// currently no request can be received to request timeout
	newIrChReq := &irChange{InputRecord: irData.IR, Reason: irChReq.CertReason, Req: irChReq}
	if irChangeReq, found := x.irChgReqBuffer[systemID]; found {
		if irChangeReq.Reason != newIrChReq.Reason {
			return fmt.Errorf("equivocating request for partition %s, reason has changed", systemID)
		}
		if types.EqualIR(irChangeReq.InputRecord, newIrChReq.InputRecord) {
			// duplicate already stored
			x.log.Debug("Duplicate IR change request, ignored", logger.Round(round))
			return nil
		}
		// At this point it is not possible to cast blame, so just log and ignore
		x.log.Debug(fmt.Sprintf("equivocating request for partition %s", systemID), logger.Round(round), logger.Data(newIrChReq.Req), logger.Data(irChangeReq))
		return fmt.Errorf("equivocating request for partition %s", systemID)
	}
	// Insert first valid request received and compare the others received against it
	x.irChgReqBuffer[systemID] = newIrChReq
	return nil
}

// IsChangeInBuffer returns true if there is a request for IR change from the partition
// in the buffer
func (x *IrReqBuffer) IsChangeInBuffer(id types.SystemID) bool {
	_, found := x.irChgReqBuffer[id]
	return found
}

// GeneratePayload generates new proposal payload from buffered IR change requests.
func (x *IrReqBuffer) GeneratePayload(round uint64, timeouts []types.SystemID, inProgress InProgressFn) *drctypes.Payload {
	payload := &drctypes.Payload{
		Requests: make([]*drctypes.IRChangeReq, 0, len(x.irChgReqBuffer)+len(timeouts)),
	}
	// first add timeout requests
	for _, id := range timeouts {
		// if there is a request for the same partition (same id) in buffer (prefer progress to timeout) or
		// if there is a change already in the pipeline for this system id
		if x.IsChangeInBuffer(id) || inProgress(id) != nil {
			x.log.Debug(fmt.Sprintf("T2 timeout request ignored, partition %s has pending change in progress", id))
			continue
		}
		x.log.Debug(fmt.Sprintf("partition %s request T2 timeout", id), logger.Round(round))
		payload.Requests = append(payload.Requests, &drctypes.IRChangeReq{
			Partition:  id,
			CertReason: drctypes.T2Timeout,
		})
	}
	for _, req := range x.irChgReqBuffer {
		if inProgress(req.Req.Partition) != nil {
			// if there is a pending block with the system id in progress then do not propose a change
			// before last has been certified
			x.log.Debug(fmt.Sprintf("partition %s request ignored, pending change in pipeline", req.Req.Partition))
			continue
		}
		payload.Requests = append(payload.Requests, req.Req)
	}
	// clear the buffer once payload is done
	clear(x.irChgReqBuffer)
	return payload
}
