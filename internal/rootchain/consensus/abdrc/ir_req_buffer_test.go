package abdrc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	mockIRVerifier struct {
	}
)

func NewAlwaysTrueIRReqVerifier() *mockIRVerifier {
	return &mockIRVerifier{}
}

func (x *mockIRVerifier) VerifyIRChangeReq(_ uint64, irChReq *abtypes.IRChangeReq) (*storage.InputData, error) {
	return &storage.InputData{SysID: irChReq.SystemIdentifier, IR: irChReq.Requests[0].InputRecord, Sdrh: []byte{0, 0, 0, 0, 1}}, nil
}

var sysID1 = types.SystemID32(1)
var sysID2 = types.SystemID32(2)
var inputRecord1 = &types.InputRecord{
	PreviousHash:    []byte{1, 1, 1},
	Hash:            []byte{2, 2, 2},
	BlockHash:       []byte{3, 3, 3},
	SummaryValue:    []byte{4, 4, 4},
	RoundNumber:     5,
	SumOfEarnedFees: 6,
}
var inputRecord2 = &types.InputRecord{
	PreviousHash:    []byte{1, 1, 1},
	Hash:            []byte{5, 5, 5},
	BlockHash:       []byte{3, 3, 3},
	SummaryValue:    []byte{4, 4, 4},
	RoundNumber:     5,
	SumOfEarnedFees: 6,
}

func TestIrReqBuffer_AddNil(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	ver := NewAlwaysTrueIRReqVerifier()
	require.ErrorContains(t, reqBuffer.Add(3, nil, ver), "ir change request is nil")
}

func TestIrReqBuffer_Add(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	ver := NewAlwaysTrueIRReqVerifier()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysID1.ToSystemID(),
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	IrChReqMsg := &abtypes.IRChangeReq{
		SystemIdentifier: sysID1,
		CertReason:       abtypes.Quorum,
		Requests:         []*certification.BlockCertificationRequest{req1},
	}
	timeouts := make([]types.SystemID32, 0, 2)
	isPending := func(id types.SystemID32) bool {
		return false
	}
	// no requests, generate payload
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Empty(t, payload.Requests)
	require.False(t, reqBuffer.IsChangeInBuffer(sysID1))
	// add request
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	// sysID1 change is in buffer
	require.True(t, reqBuffer.IsChangeInBuffer(sysID1))
	// try to add the same again, considered duplicate no error
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	// change reason and try to add, must be rejected as equivocating, we already have a valid request
	IrChReqMsg.CertReason = abtypes.T2Timeout
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"invalid ir change request, timeout can only be proposed by leader issuing a new block")
	// change IR and try to add, must be rejected as equivocating, we already have a valid request
	IrChReqMsg.CertReason = abtypes.Quorum
	IrChReqMsg.Requests[0].InputRecord = inputRecord2
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"equivocating request for partition 00000001")
	// try to change reason
	IrChReqMsg.CertReason = abtypes.QuorumNotPossible
	IrChReqMsg.Requests[0].InputRecord = inputRecord1
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"equivocating request for partition 00000001, reason has changed")
	// Generate proposal payload, one request in buffer
	payload = reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 1)
	// generate payload again, but now it is empty
	payloadNowEmpty := reqBuffer.GeneratePayload(4, timeouts, isPending)
	require.Empty(t, payloadNowEmpty.Requests)
	require.False(t, reqBuffer.IsChangeInBuffer(sysID1))
	// finally verify that we got the original message back
	require.Equal(t, IrChReqMsg, payload.Requests[0])
}

func TestIrReqBuffer_TimeoutReq(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	timeouts := []types.SystemID32{sysID1, sysID2}
	isPending := func(id types.SystemID32) bool {
		return false
	}
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 2)
	// if both then prefer to make progress over timeout
	require.Equal(t, sysID1, payload.Requests[0].SystemIdentifier)
	require.Equal(t, abtypes.T2Timeout, payload.Requests[0].CertReason)
	require.Empty(t, payload.Requests[0].Requests)
	require.Equal(t, sysID2, payload.Requests[1].SystemIdentifier)
	require.Equal(t, abtypes.T2Timeout, payload.Requests[1].CertReason)
	require.Empty(t, payload.Requests[1].Requests)
}

func TestIrReqBuffer_TimeoutAndNewReq(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	ver := NewAlwaysTrueIRReqVerifier()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysID1.ToSystemID(),
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	IrChReqMsg := &abtypes.IRChangeReq{
		SystemIdentifier: sysID1,
		CertReason:       abtypes.Quorum,
		Requests:         []*certification.BlockCertificationRequest{req1},
	}
	timeouts := []types.SystemID32{sysID1}
	isPending := func(id types.SystemID32) bool {
		return false
	}
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 1)
	// if both then prefer to make progress over timeout
	require.Equal(t, abtypes.Quorum, payload.Requests[0].CertReason)
}

func TestIrReqBuffer_TimeoutAndReqButAChangeIsPending(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	ver := NewAlwaysTrueIRReqVerifier()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysID1.ToSystemID(),
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	IrChReqMsg := &abtypes.IRChangeReq{
		SystemIdentifier: sysID1,
		CertReason:       abtypes.Quorum,
		Requests:         []*certification.BlockCertificationRequest{req1},
	}
	timeouts := []types.SystemID32{sysID1}
	isPending := func(id types.SystemID32) bool {
		return true
	}
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 0)
}
