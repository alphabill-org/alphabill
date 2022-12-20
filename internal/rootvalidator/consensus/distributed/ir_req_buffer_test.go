package distributed

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
)

/*
	func createUC(round uint64, sysId, hash []byte) *certificates.UnicityCertificate {
		return &certificates.UnicityCertificate{
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      sysId,
				SiblingHashes:         nil,
				SystemDescriptionHash: nil,
			},
			UnicitySeal: &certificates.UnicitySeal{
				RootChainRoundNumber: round,
				PreviousHash:         make([]byte, gocrypto.SHA256.Size()),
				Hash:                 hash,
				RoundCreationTime:    util.MakeTimestamp(),
			},
		}
	}

	func createIRChangeRequest(sysId []byte, reason atomic_broadcast.IRChangeReqMsg_CERT_REASON, proof ...*certification.BlockCertificationRequest) *atomic_broadcast.IRChangeReqMsg {
		return &atomic_broadcast.IRChangeReqMsg{
			SystemIdentifier: sysId,
			CertReason: reason,
			Requests: proof,
		}
	}

	func TestIrReqBuffer_Add(t *testing.T) {
		var sysId0 = []byte{0, 0, 0, 0}
		var sysId1 = []byte{0, 0, 0, 1}
		var lastHash = []byte{1, 1, 1, 1}
		irBuffer := NewIrReqBuffer()
		luc := createUC(3, sysId0, lastHash)
		// add invalid system identifier
		req := &certification.BlockCertificationRequest{}
		req := createIRChangeRequest(sysId1, atomic_broadcast.IRChangeReqMsg_QUORUM, )
		err := irBuffer.Add(, luc)
	}
*/
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
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
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
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
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
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
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
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
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
				a: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1},
					Hash:         []byte{2, 2, 2},
					BlockHash:    []byte{3, 3, 3},
					SummaryValue: []byte{4, 4, 4}},
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
