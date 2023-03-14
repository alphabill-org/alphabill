package atomic_broadcast

import (
	gocrypto "crypto"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	certification "github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partitions"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

var sysId1 = []byte{0, 0, 0, 1}
var sysId2 = []byte{0, 0, 0, 2}
var prevHash = []byte{0, 0, 0}
var inputRecord1 = &certificates.InputRecord{
	PreviousHash: prevHash,
	Hash:         []byte{1, 1, 1},
	BlockHash:    []byte{2, 2, 2},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  2,
}
var inputRecord2 = &certificates.InputRecord{
	PreviousHash: prevHash,
	Hash:         []byte{5, 5, 5},
	BlockHash:    []byte{2, 2, 2},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  2,
}

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
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
							RoundNumber:  1,
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
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
							RoundNumber:  1,
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
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
							RoundNumber:  1,
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
		'1',                                            // node identifier - string is encoded without '/0'
		0, 1, 2, 3, 4, 5, 6, 7, 0, 0, 0, 0, 0, 0, 0, 3, // prev. hash, hash, block hash, summary value, round number
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
						InputRecord: &certificates.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
							RoundNumber:  3,
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

func TestIRChangeReqMsg_GeneralReq(t *testing.T) {
	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		tb  partitions.PartitionTrustBase
		luc *certificates.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)

	trustBase := &partitions.TrustBase{
		PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2}}
	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS1.Sign(s1))
	reqS2 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "2",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS2.Sign(s2))

	invalidReq := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
		Signature:        []byte{0, 19, 12},
	}

	reqS1InvalidSysId := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId2,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS1InvalidSysId.Sign(s1))

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
				tb: trustBase,
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash},
				},
			},
			wantErrStr: "invalid ir change request, invalid system identifier",
		},
		{
			name: "System id in request and proof do not match",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1InvalidSysId, reqS2},
			},
			args: args{
				tb: trustBase,
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid ir change request proof, 00000001 node 1 proof system id 00000002 does not match",
		},
		{
			name: "IR change request, signature verification error",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{invalidReq},
			},
			args: args{
				tb: trustBase,
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "signature verification failed",
		},
		{
			name: "Proof contains more request, than there are partition nodes",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid ir change request, proof contains more requests than registered partition nodes",
		},
		{
			name: "Proof contains more request from unknown node",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "3": v2},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid ir change request, proof from system id 00000001 node 2 is not valid: verification failed, unknown node id 2",
		},
		{
			name: "Proof contains duplicate node",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid ir change request proof, partition 00000001 proof contains duplicate request from node 1",
		},
		{
			name: "IR does not extend last certified state",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 0, 1}, RoundNumber: 1},
				},
			},
			wantErrStr: "input records does not extend last certified",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqMsg{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			ir, err := x.Verify(tt.args.tb, tt.args.luc, 0, 0)
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

func TestIRChangeReqMsg_VerifyTimeoutReq(t *testing.T) {
	type fields struct {
		SystemIdentifier []byte
		CertReason       IRChangeReqMsg_CERT_REASON
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		tb         partitions.PartitionTrustBase
		luc        *certificates.UnicityCertificate
		t2InRounds uint64
		round      uint64
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS1.Sign(s1))
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "IR change request in timeout proof contains requests, not valid",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
					UnicitySeal: &certificates.UnicitySeal{
						RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 1},
					},
				},
			},
			wantErrStr: "invalid ir change request timeout proof, proof contains requests",
		},
		{
			name: "Invalid, not yet timeout",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         nil,
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash},
					UnicitySeal: &certificates.UnicitySeal{
						RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 1},
					},
				},
				t2InRounds: 5,
				round:      5,
			},
			wantErrStr: "invalid ir change request timeout proof, partition 00000001 time from latest UC 4, timeout in rounds 5",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_T2_TIMEOUT,
				Requests:         nil,
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash},
					UnicitySeal: &certificates.UnicitySeal{
						RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 1},
					},
				},
				t2InRounds: 5,
				round:      6,
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
			ir, err := x.Verify(tt.args.tb, tt.args.luc, tt.args.round, tt.args.t2InRounds)
			if tt.wantErrStr == "" {
				require.NoError(t, err)
				repeatIR, _ := certificates.NewRepeatInputRecord(tt.args.luc.InputRecord)
				require.Equal(t, repeatIR, ir)
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
		tb  partitions.PartitionTrustBase
		luc *certificates.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)

	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS1.Sign(s1))
	reqS2 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "2",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS2.Sign(s2))
	reqS3NotMatchingIR := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "3",
		InputRecord:      inputRecord2,
	}
	require.NoError(t, reqS3NotMatchingIR.Sign(s3))

	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Not enough requests for quorum",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "not enough requests to prove quorum",
		},
		{
			name: "IR does not extend last certified state",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: []byte{0, 0, 1}, RoundNumber: 1},
				},
			},
			wantErrStr: "input records does not extend last certified",
		},
		{
			name: "Contains not matching proofs",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2, reqS3NotMatchingIR},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid ir change request quorum proof, partition 00000001 contains proofs for no quorum",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
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
			ir, err := x.Verify(tt.args.tb, tt.args.luc, 0, 0)
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
		tb  partitions.PartitionTrustBase
		luc *certificates.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)

	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1,
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS1.Sign(s1))
	reqS2 := &certification.BlockCertificationRequest{
		SystemIdentifier: []byte{0, 0, 0, 1},
		NodeIdentifier:   "2",
		InputRecord:      inputRecord2,
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
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "not enough requests to prove no quorum is possible",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       IRChangeReqMsg_QUORUM_NOT_POSSIBLE,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &certificates.UnicityCertificate{
					InputRecord: &certificates.InputRecord{Hash: prevHash, RoundNumber: 1},
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
			ir, err := x.Verify(tt.args.tb, tt.args.luc, 0, 0)
			if tt.wantErrStr == "" {
				require.NoError(t, err)
				repeatIR, _ := certificates.NewRepeatInputRecord(tt.args.luc.InputRecord)
				require.Equal(t, repeatIR, ir)
				return
			}
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, ir)
		})
	}
}
