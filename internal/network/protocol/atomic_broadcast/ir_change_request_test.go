package atomic_broadcast

import (
	gocrypto "crypto"
	"hash"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	certification "github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestIRChangeReqMsg_IsValid(t *testing.T) {

	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	tests := []struct {
		name       string
		fields     fields
		wantErr    bool
		serialized []byte
	}{
		{
			name: "IR change req. invalid system id",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 1},
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 1},
						NodeIdentifier:   "1",
						RootRoundNumber:  9,
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
						},
						Signature: []byte{0, 1},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "IR change req. invalid cert reason",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       -2,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 2},
						NodeIdentifier:   "1",
						RootRoundNumber:  9,
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
						},
						Signature: []byte{0, 1},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "IR change req. invalid cert reason",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       10,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 2},
						NodeIdentifier:   "1",
						RootRoundNumber:  9,
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
						},
						Signature: []byte{0, 1},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			if err := x.IsValid(); (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIRChangeReqMsg_AddToHasher(t *testing.T) {
	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		hasher hash.Hash
	}
	expectedHash := gocrypto.SHA256.New()

	expectedHash.Write([]byte{
		0, 0, 0, 1, // 4 byte System identifier of IRChangeReqMsg
		0, 0, 0, 1, // cert reason quorum not possible
		// Start of the BlockCertificationRequest
		0, 0, 0, 1, // 4 byte system identifier
		'1',                    // node identifier - string is encoded without '/0'
		0, 0, 0, 0, 0, 0, 0, 9, // round number
		0, 1, 2, 3, 4, 5, 6, 7, // prev. hash, hash, block hash, summary value
		// Is serialized without signature
	})

	tests := []struct {
		name     string
		fields   fields
		args     args
		wantHash []byte
	}{
		{
			name: "Bytes result",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 1},
						NodeIdentifier:   "1",
						RootRoundNumber:  9,
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
						},
						Signature: []byte{0, 1},
					},
				},
			},
			args:     args{hasher: gocrypto.SHA256.New()},
			wantHash: expectedHash.Sum(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			x.AddToHasher(tt.args.hasher)
			require.Equal(t, tt.args.hasher.Sum(nil), tt.wantHash[:])
		})
	}
}

func TestIRChangeReqMsg_Verify(t *testing.T) {
	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		partitionInfo partition_store.PartitionInfo
		luc           *certificates.UnicityCertificate
		lucAge        time.Duration
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]crypto.Verifier{"1": v1}
	req := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{},
			Hash:         []byte{},
			BlockHash:    []byte{},
			SummaryValue: []byte{},
		},
	}
	invalidReq := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{},
			Hash:         []byte{},
			BlockHash:    []byte{},
			SummaryValue: []byte{},
		},
		Signature: []byte{0, 19, 12},
	}
	require.NoError(t, req.Sign(s1))

	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "IR change request timeout is not valid",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         []*certification.BlockCertificationRequest{req},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 1}},
				},
				lucAge: 0,
			},
			wantErrStr: "invalid ir change request timeout proof, proof contains requests",
		},
		{
			name: "IR does not extend last certified state",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{req},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 1}},
				},
				lucAge: 0,
			},
			wantErrStr: "input records does not extend last certified",
		},
		{
			name: "IR change request verify OK",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests:         []*certification.BlockCertificationRequest{invalidReq},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 1}},
				},
				lucAge: 0,
			},
			wantErrStr: "invalid ir change request, signature validation error",
		},
		{
			name: "No proof quorum not possible",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests:         []*certification.BlockCertificationRequest{req},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 1}},
				},
				lucAge: 0,
			},
			wantErrStr: "not enough requests to prove no quorum is possible",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			ir, err := x.Verify(tt.args.partitionInfo, tt.args.luc, tt.args.lucAge)
			if tt.wantErrStr == "" {
				require.NoError(t, err)
				require.NotNil(t, ir)
				return
			}
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, ir)
		})
	}
}
