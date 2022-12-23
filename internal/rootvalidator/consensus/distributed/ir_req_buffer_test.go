package distributed

import (
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
)

var sysId1 = []byte{0, 0, 0, 1}
var sysId2 = []byte{0, 0, 0, 2}
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

func TestIrReqBuffer_Add(t *testing.T) {
	reqBuffer := NewIrReqBuffer()
	// add a request that reached consensus
	req1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "1",
		RootRoundNumber:  9,
		InputRecord:      inputRecord1,
	}
	IrChReqMsg := &atomic_broadcast.IRChangeReqMsg{
		SystemIdentifier: sysId1,
		CertReason:       atomic_broadcast.IRChangeReqMsg_QUORUM,
		Requests:         []*certification.BlockCertificationRequest{req1},
	}
	// no requests, generate payload
	payload := reqBuffer.GeneratePayload()
	require.Empty(t, payload.Requests)
	// add request
	require.NoError(t, reqBuffer.Add(IRChange{InputRecord: inputRecord1, Reason: IrChReqMsg.CertReason, Msg: IrChReqMsg}))
	// try to add the same again, considered duplicate no error
	require.NoError(t, reqBuffer.Add(IRChange{InputRecord: inputRecord1, Reason: IrChReqMsg.CertReason, Msg: IrChReqMsg}))
	// change reason and try to add, must be rejected as equivocating, we already have a valid request
	require.ErrorContains(t, reqBuffer.Add(IRChange{InputRecord: inputRecord1, Reason: atomic_broadcast.IRChangeReqMsg_T2_TIMEOUT, Msg: IrChReqMsg}),
		"error equivocating request for partition 00000001 reason has changed")
	// change IR and try to add, must be rejected as equivocating, we already have a valid request
	require.ErrorContains(t, reqBuffer.Add(IRChange{InputRecord: inputRecord2, Reason: IrChReqMsg.CertReason, Msg: IrChReqMsg}),
		"error equivocating request for partition 00000001")
	// Generate proposal payload, one request in buffer
	payload = reqBuffer.GeneratePayload()
	require.Len(t, payload.Requests, 1)
	// generate payload again, but now it is empty
	payloadNowEmpty := reqBuffer.GeneratePayload()
	require.Empty(t, payloadNowEmpty.Requests)
	// finally verify that we got the original message back
	require.Equal(t, IrChReqMsg, payload.Requests[0])
}
