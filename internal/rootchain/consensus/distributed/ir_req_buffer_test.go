package distributed

import (
	"testing"

	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/distributed/storage"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/certificates"
)

type (
	mockTimeoutGen struct {
		timeoutIds []p.SystemIdentifier
	}

	mockIRVerifier struct {
	}
)

func NewMockTimeoutGenerator() *mockTimeoutGen {
	return &mockTimeoutGen{
		timeoutIds: make([]p.SystemIdentifier, 0),
	}
}

func (x *mockTimeoutGen) setTimeout(id p.SystemIdentifier) {
	x.timeoutIds = append(x.timeoutIds, id)
}

func (x *mockTimeoutGen) clear() {
	x.timeoutIds = make([]p.SystemIdentifier, 0)
}

func (x *mockTimeoutGen) GetT2Timeouts(_ uint64) []p.SystemIdentifier {
	return x.timeoutIds
}

func NewAlwaysTrueIRReqVerifier() *mockIRVerifier {
	return &mockIRVerifier{}
}

func (x *mockIRVerifier) VerifyIRChangeReq(_ uint64, irChReq *ab_consensus.IRChangeReqMsg) (*storage.InputData, error) {
	return &storage.InputData{SysID: irChReq.SystemIdentifier, IR: irChReq.Requests[0].InputRecord, Sdrh: []byte{0, 0, 0, 0, 1}}, nil
}

var sysID1 = []byte{0, 0, 0, 1}
var sysID2 = []byte{0, 0, 0, 2}
var inputRecord1 = &certificates.InputRecord{
	PreviousHash: []byte{1, 1, 1},
	Hash:         []byte{2, 2, 2},
	BlockHash:    []byte{3, 3, 3},
	SummaryValue: []byte{4, 4, 4},
}
var inputRecord2 = &certificates.InputRecord{
	PreviousHash: []byte{1, 1, 1},
	Hash:         []byte{5, 5, 5},
	BlockHash:    []byte{3, 3, 3},
	SummaryValue: []byte{4, 4, 4},
}

func Test_compareIR(t *testing.T) {
	type args struct {
		a *certificates.InputRecord
		b *certificates.InputRecord
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "equal",
			args: args{
				a: inputRecord1,
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
			},
			want: true,
		},
		{
			name: "Previous hash not equal",
			args: args{
				a: inputRecord1,
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
			},
			want: false,
		},
		{
			name: "Hash not equal",
			args: args{
				a: inputRecord1,
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2, 3},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
			},
			want: false,
		},
		{
			name: "Block hash not equal",
			args: args{
				a: inputRecord1,
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    nil,
					SummaryValue: []byte{4, 4, 4}},
			},
			want: false,
		},
		{
			name: "Summary value not equal",
			args: args{
				a: inputRecord1,
				b: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareIR(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("compareIR() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIrReqBuffer_AddNil(t *testing.T) {
	reqBuffer := NewIrReqBuffer()
	ver := NewAlwaysTrueIRReqVerifier()
	require.ErrorContains(t, reqBuffer.Add(3, nil, ver), "ir change request is nil")
}

func TestIrReqBuffer_Add(t *testing.T) {
	reqBuffer := NewIrReqBuffer()
	mockTimeouts := NewMockTimeoutGenerator()
	ver := NewAlwaysTrueIRReqVerifier()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysID1,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	IrChReqMsg := &ab_consensus.IRChangeReqMsg{
		SystemIdentifier: sysID1,
		CertReason:       ab_consensus.IRChangeReqMsg_QUORUM,
		Requests:         []*certification.BlockCertificationRequest{req1},
	}
	// no requests, generate payload
	payload := reqBuffer.GeneratePayload(3, mockTimeouts)
	require.Empty(t, payload.Requests)
	require.False(t, reqBuffer.IsChangeInBuffer(p.SystemIdentifier(sysID1)))
	// add request
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	// sysID1 change is in buffer
	require.True(t, reqBuffer.IsChangeInBuffer(p.SystemIdentifier(sysID1)))
	// try to add the same again, considered duplicate no error
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	// change reason and try to add, must be rejected as equivocating, we already have a valid request
	IrChReqMsg.CertReason = ab_consensus.IRChangeReqMsg_T2_TIMEOUT
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"invalid ir change request, timeout can only be proposed by leader issuing a new block")
	// change IR and try to add, must be rejected as equivocating, we already have a valid request
	IrChReqMsg.CertReason = ab_consensus.IRChangeReqMsg_QUORUM
	IrChReqMsg.Requests[0].InputRecord = inputRecord2
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"error equivocating request for partition 00000001")
	// try to change reason
	IrChReqMsg.CertReason = ab_consensus.IRChangeReqMsg_QUORUM_NOT_POSSIBLE
	IrChReqMsg.Requests[0].InputRecord = inputRecord1
	require.ErrorContains(t, reqBuffer.Add(3, IrChReqMsg, ver),
		"error equivocating request for partition 00000001 reason has changed")
	// Generate proposal payload, one request in buffer
	payload = reqBuffer.GeneratePayload(3, mockTimeouts)
	require.Len(t, payload.Requests, 1)
	// generate payload again, but now it is empty
	payloadNowEmpty := reqBuffer.GeneratePayload(4, mockTimeouts)
	require.Empty(t, payloadNowEmpty.Requests)
	require.False(t, reqBuffer.IsChangeInBuffer(p.SystemIdentifier(sysID1)))
	// finally verify that we got the original message back
	require.Equal(t, IrChReqMsg, payload.Requests[0])
}

func TestIrReqBuffer_TimeoutReq(t *testing.T) {
	reqBuffer := NewIrReqBuffer()
	mockTimeouts := NewMockTimeoutGenerator()
	mockTimeouts.setTimeout(p.SystemIdentifier(sysID1))
	mockTimeouts.setTimeout(p.SystemIdentifier(sysID2))
	payload := reqBuffer.GeneratePayload(3, mockTimeouts)
	require.Len(t, payload.Requests, 2)
	// if both then prefer to make progress over timeout
	require.Equal(t, sysID1, payload.Requests[0].SystemIdentifier)
	require.Equal(t, ab_consensus.IRChangeReqMsg_T2_TIMEOUT, payload.Requests[0].CertReason)
	require.Empty(t, payload.Requests[0].Requests)
	require.Equal(t, sysID2, payload.Requests[1].SystemIdentifier)
	require.Equal(t, ab_consensus.IRChangeReqMsg_T2_TIMEOUT, payload.Requests[1].CertReason)
	require.Empty(t, payload.Requests[1].Requests)
}

func TestIrReqBuffer_TimeoutAndNewReq(t *testing.T) {
	reqBuffer := NewIrReqBuffer()
	mockTimeouts := NewMockTimeoutGenerator()
	ver := NewAlwaysTrueIRReqVerifier()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysID1,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	IrChReqMsg := &ab_consensus.IRChangeReqMsg{
		SystemIdentifier: sysID1,
		CertReason:       ab_consensus.IRChangeReqMsg_QUORUM,
		Requests:         []*certification.BlockCertificationRequest{req1},
	}
	mockTimeouts.setTimeout(p.SystemIdentifier(sysID1))
	require.NoError(t, reqBuffer.Add(3, IrChReqMsg, ver))
	payload := reqBuffer.GeneratePayload(3, mockTimeouts)
	require.Len(t, payload.Requests, 1)
	// if both then prefer to make progress over timeout
	require.Equal(t, ab_consensus.IRChangeReqMsg_QUORUM, payload.Requests[0].CertReason)
}
