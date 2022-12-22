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

func TestIRChangeReqMsg_VerifyTimeoutReq(t *testing.T) {
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
	reqS1 := &certification.BlockCertificationRequest{
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
	require.NoError(t, reqS1.Sign(s1))
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Not valid system id",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 1},
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         nil,
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
			wantErrStr: "invalid ir change request, invalid system identifier",
		},
		{
			name: "IR change request timeout is not valid",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
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
			name: "Invalid, not yet timeout",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         nil,
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
				lucAge: time.Duration(2499) * time.Millisecond,
			},
			wantErrStr: "invalid ir change request timeout proof, partition 00000001 time from latest UC 2.499s",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         nil,
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
				lucAge: time.Duration(2500) * time.Millisecond,
			},
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
				require.Equal(t, tt.args.luc.InputRecord, ir)
				return
			}
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, ir)
		})
	}
}

func TestIRChangeReqMsg_VerifyQuorum(t *testing.T) {
	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		partitionInfo partition_store.PartitionInfo
		luc           *certificates.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)

	trustBase := map[string]crypto.Verifier{"1": v1, "2": v2}
	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{0, 0, 0},
			Hash:         []byte{1, 1, 1},
			BlockHash:    []byte{2, 2, 2},
			SummaryValue: []byte{4, 4, 4},
		},
	}
	require.NoError(t, reqS1.Sign(s1))
	reqS2 := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "2",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{0, 0, 0},
			Hash:         []byte{1, 1, 1},
			BlockHash:    []byte{2, 2, 2},
			SummaryValue: []byte{4, 4, 4},
		},
	}
	require.NoError(t, reqS2.Sign(s2))

	invalidReq := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{0, 0, 0},
			Hash:         []byte{1, 1, 1},
			BlockHash:    []byte{2, 2, 2},
			SummaryValue: []byte{4, 4, 4},
		},
		Signature: []byte{0, 19, 12},
	}

	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "IR change request, signature verification error",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM,
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
			},
			wantErrStr: "signature verification failed",
		},
		{
			name: "Proof contains more request, than there are partition nodes",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: map[string]crypto.Verifier{"1": v1}},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 1}},
				},
			},
			wantErrStr: "invalid ir change request, proof contains more requests than registered partition nodes",
		},
		{
			name: "Not enough requests for quorum",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 0, 0}},
				},
			},
			wantErrStr: "not enough requests to prove quorum",
		},
		{
			name: "IR does not extend last certified state",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
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
			},
			wantErrStr: "input records does not extend last certified",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 0, 0}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			ir, err := x.Verify(tt.args.partitionInfo, tt.args.luc, 0)
			if tt.wantErrStr == "" {
				require.NoError(t, err)
				require.Equal(t, tt.fields.Requests[0].InputRecord, ir)
				return
			}
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, ir)
		})
	}
}

func TestIRChangeReqMsg_VerifyQuorumNotPossible(t *testing.T) {
	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		partitionInfo partition_store.PartitionInfo
		luc           *certificates.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)

	trustBase := map[string]crypto.Verifier{"1": v1, "2": v2}
	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "1",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{0, 0, 0},
			Hash:         []byte{1, 1, 1},
			BlockHash:    []byte{2, 2, 2},
			SummaryValue: []byte{4, 4, 4},
		},
	}
	require.NoError(t, reqS1.Sign(s1))
	reqS2 := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "2",
		RootRoundNumber:  0,
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{0, 0, 0},
			Hash:         []byte{5, 5, 5},
			BlockHash:    []byte{2, 2, 2},
			SummaryValue: []byte{4, 4, 4},
		},
	}
	require.NoError(t, reqS2.Sign(s2))

	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "No proof quorum not possible",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 0, 0}},
				},
			},
			wantErrStr: "not enough requests to prove no quorum is possible",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: []byte{0, 0, 0, 1},
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				partitionInfo: partition_store.PartitionInfo{
					SystemDescription: &genesis.SystemDescriptionRecord{
						SystemIdentifier: []byte{0, 0, 0, 1},
						T2Timeout:        2500,
					},
					TrustBase: trustBase},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 0, 0}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			ir, err := x.Verify(tt.args.partitionInfo, tt.args.luc, 0)
			if tt.wantErrStr == "" {
				require.NoError(t, err)
				require.Equal(t, tt.args.luc.InputRecord, ir)
				return
			}
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, ir)
		})
	}
}
