package distributed

import (
	"bytes"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/request_store"
)

type (
	IRChange struct {
		InputRecord *certificates.InputRecord
		Reason      atomic_broadcast.IRChangeReqMsg_CERT_REASON
		Msg         *atomic_broadcast.IRChangeReqMsg
	}
	ProposalGenerator struct {
		irChgReqBuffer map[protocol.SystemIdentifier][]*IRChange
	}
)

func NewProposalGenerator() *ProposalGenerator {
	return &ProposalGenerator{
		irChgReqBuffer: make(map[protocol.SystemIdentifier][]*IRChange),
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

func (x *ProposalGenerator) ValidateAndBufferIRReq(req *atomic_broadcast.IRChangeReqMsg, luc *certificates.UnicityCertificate, info *partition_store.PartitionInfo) error {
	systemId := protocol.SystemIdentifier(req.SystemIdentifier)

	irChangeReqs, found := x.irChgReqBuffer[systemId]
	// If there is a pending request, then compare and complain if not duplicate
	requestStore := request_store.NewCertificationRequestStore()
	for _, r := range req.Requests {
		// check request against last state, filter out:
		// 1. requests that extend invalid or old state
		// 2. requests that are stale
		if err := consensus.CheckBlockCertificationRequest(r, luc); err != nil {
			logger.Warning("Invalid IR change request: %v", err)
			continue
		}
		if err := requestStore.Add(r); err != nil {
			logger.Warning("Invalid IR change request: %v", err)
			continue
		}
	}
	ir, _ := requestStore.IsConsensusReceived(systemId, len(info.TrustBase))
	if ir == nil {
		logger.Warning("Invalid IR change request no consensus reached")
		return errors.New("invalid IR change request no consensus reached")
	}
	//todo: check for pending requests
	if found {
		// compare IR's
		if compareIR(irChangeReqs[0].InputRecord, ir) == true {
			logger.Debug("Duplicate IR change request, ignored")
			return nil
		}
		// store evidence of equivocating request
		irChangeReqs = append(irChangeReqs, &IRChange{InputRecord: ir, Reason: req.CertReason, Msg: req})
		return nil
	}
	// Insert first valid request received and compare the others received against it
	x.irChgReqBuffer[systemId] = []*IRChange{{InputRecord: ir, Reason: req.CertReason, Msg: req}}
	return nil
}

func (x *ProposalGenerator) GetPayload() *atomic_broadcast.Payload {
	payload := &atomic_broadcast.Payload{
		Requests: make([]*atomic_broadcast.IRChangeReqMsg, len(x.irChgReqBuffer)),
	}
	i := 0
	for _, req := range x.irChgReqBuffer {
		// If there is more than one request stored, there is an equivocating request too, skip those
		// todo: logging of equivocating requests with evidence - separate log file or DB?
		if len(req) == 1 {
			payload.Requests[i] = req[0].Msg
		}
		i++
	}
	return payload
}
