package consensus

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/stretchr/testify/require"
)

type (
	mockIRVerifier struct {
	}
)

func NewAlwaysTrueIRReqVerifier() *mockIRVerifier {
	return &mockIRVerifier{}
}

func (x *mockIRVerifier) VerifyIRChangeReq(_ uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error) {
	return &storage.InputData{Partition: irChReq.Partition, IR: irChReq.Requests[0].InputRecord, PDRHash: []byte{0, 0, 0, 0, 1}}, nil
}

const partitionID1 types.PartitionID = 1
const partitionID2 types.PartitionID = 2

var inputRecord1 = &types.InputRecord{
	Version:         1,
	PreviousHash:    []byte{1, 1, 1},
	Hash:            []byte{2, 2, 2},
	BlockHash:       []byte{3, 3, 3},
	SummaryValue:    []byte{4, 4, 4},
	RoundNumber:     5,
	SumOfEarnedFees: 6,
}
var inputRecord2 = &types.InputRecord{
	Version:         1,
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
		PartitionID: partitionID1,
		NodeID:      "1",
		InputRecord: inputRecord1,
	}
	IrChReqMsg := &drctypes.IRChangeReq{
		Partition:  partitionID1,
		CertReason: drctypes.Quorum,
		Requests:   []*certification.BlockCertificationRequest{req1},
	}
	timeouts := make([]types.PartitionID, 0, 2)
	isPending := func(id types.PartitionID, _ types.ShardID) *types.InputRecord {
		return nil
	}
	// no requests, generate payload
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Empty(t, payload.Requests)
	require.False(t, reqBuffer.IsChangeInBuffer(partitionID1))
	// add request
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	// sysID1 change is in buffer
	require.True(t, reqBuffer.IsChangeInBuffer(partitionID1))
	// try to add the same again, considered duplicate no error
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	// change reason and try to add, must be rejected as equivocating, we already have a valid request
	IrChReqMsg.CertReason = drctypes.T2Timeout
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"invalid ir change request, timeout can only be proposed by leader issuing a new block")
	// change IR and try to add, must be rejected as equivocating, we already have a valid request
	IrChReqMsg.CertReason = drctypes.Quorum
	IrChReqMsg.Requests[0].InputRecord = inputRecord2
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"equivocating request for partition 00000001")
	// try to change reason
	IrChReqMsg.CertReason = drctypes.QuorumNotPossible
	IrChReqMsg.Requests[0].InputRecord = inputRecord1
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"equivocating request for partition 00000001, reason has changed")
	// Generate proposal payload, one request in buffer
	payload = reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 1)
	// generate payload again, but now it is empty
	payloadNowEmpty := reqBuffer.GeneratePayload(4, timeouts, isPending)
	require.Empty(t, payloadNowEmpty.Requests)
	require.False(t, reqBuffer.IsChangeInBuffer(partitionID1))
	// finally verify that we got the original message back
	require.Equal(t, IrChReqMsg, payload.Requests[0])
}

func TestIrReqBuffer_TimeoutReq(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	timeouts := []types.PartitionID{partitionID1, partitionID2}
	isPending := func(id types.PartitionID, _ types.ShardID) *types.InputRecord {
		return nil
	}
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 2)
	// if both then prefer to make progress over timeout
	require.Equal(t, partitionID1, payload.Requests[0].Partition)
	require.Equal(t, drctypes.T2Timeout, payload.Requests[0].CertReason)
	require.Empty(t, payload.Requests[0].Requests)
	require.Equal(t, partitionID2, payload.Requests[1].Partition)
	require.Equal(t, drctypes.T2Timeout, payload.Requests[1].CertReason)
	require.Empty(t, payload.Requests[1].Requests)
}

func TestIrReqBuffer_TimeoutAndNewReq(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	ver := NewAlwaysTrueIRReqVerifier()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		PartitionID: partitionID1,
		NodeID:      "1",
		InputRecord: inputRecord1,
	}
	IrChReqMsg := &drctypes.IRChangeReq{
		Partition:  partitionID1,
		CertReason: drctypes.Quorum,
		Requests:   []*certification.BlockCertificationRequest{req1},
	}
	timeouts := []types.PartitionID{partitionID1}
	isPending := func(id types.PartitionID, _ types.ShardID) *types.InputRecord {
		return nil
	}
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 1)
	// if both then prefer to make progress over timeout
	require.Equal(t, drctypes.Quorum, payload.Requests[0].CertReason)
}

func TestIrReqBuffer_TimeoutAndReqButAChangeIsPending(t *testing.T) {
	reqBuffer := NewIrReqBuffer(logger.New(t))
	ver := NewAlwaysTrueIRReqVerifier()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		PartitionID: partitionID1,
		NodeID:      "1",
		InputRecord: inputRecord1,
	}
	IrChReqMsg := &drctypes.IRChangeReq{
		Partition:  partitionID1,
		CertReason: drctypes.Quorum,
		Requests:   []*certification.BlockCertificationRequest{req1},
	}
	timeouts := []types.PartitionID{partitionID1}
	isPending := func(id types.PartitionID, _ types.ShardID) *types.InputRecord {
		return &types.InputRecord{Version: 1}
	}
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	payload := reqBuffer.GeneratePayload(3, timeouts, isPending)
	require.Len(t, payload.Requests, 0)
}
