package types

import (
	gocrypto "crypto"
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

var sysId1 = types.SystemID32(1)
var sysId2 = types.SystemID32(2)
var prevHash = []byte{0, 0, 0}
var inputRecord1 = &types.InputRecord{
	PreviousHash: prevHash,
	Hash:         []byte{1, 1, 1},
	BlockHash:    []byte{2, 2, 2},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  2,
}
var inputRecord2 = &types.InputRecord{
	PreviousHash: prevHash,
	Hash:         []byte{5, 5, 5},
	BlockHash:    []byte{2, 2, 2},
	SummaryValue: []byte{4, 4, 4},
	RoundNumber:  2,
}

func TestIRChangeReqMsg_IsValid(t *testing.T) {

	type fields struct {
		SystemIdentifier types.SystemID32
		CertReason       IRChangeReason
		Requests         []*certification.BlockCertificationRequest
	}
	tests := []struct {
		name       string
		fields     fields
		wantErr    bool
		serialized []byte
	}{
		{
			name: "IR change req. invalid cert reason",
			fields: fields{
				SystemIdentifier: types.SystemID32(1),
				CertReason:       -2,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 2},
						NodeIdentifier:   "1",
						InputRecord: &types.InputRecord{
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
				SystemIdentifier: types.SystemID32(1),
				CertReason:       10,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 2},
						NodeIdentifier:   "1",
						InputRecord: &types.InputRecord{
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
			x := &IRChangeReq{
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
		SystemIdentifier types.SystemID32
		CertReason       IRChangeReason
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
		0, 0, 0, 0, 0, 0, 0, 2, //root round number
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
				SystemIdentifier: types.SystemID32(1),
				CertReason:       QuorumNotPossible,
				Requests: []*certification.BlockCertificationRequest{
					{
						SystemIdentifier: []byte{0, 0, 0, 1},
						NodeIdentifier:   "1",
						InputRecord: &types.InputRecord{
							PreviousHash: []byte{0, 1},
							Hash:         []byte{2, 3},
							BlockHash:    []byte{4, 5},
							SummaryValue: []byte{6, 7},
							RoundNumber:  3,
						},
						RootRoundNumber: 2,
						Signature:       []byte{0, 1},
					},
				},
			},
			args:     args{hasher: gocrypto.SHA256.New()},
			wantHash: expectedHash.Sum(nil),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReq{
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
		SystemIdentifier types.SystemID32
		CertReason       IRChangeReason
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		tb  partitions.PartitionTrustBase
		luc *types.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)

	trustBase := &partitions.TrustBase{
		PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2}}
	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS1.Sign(s1))
	reqS2 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
		NodeIdentifier:   "2",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS2.Sign(s2))

	invalidReq := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
		Signature:        []byte{0, 19, 12},
	}

	reqS1InvalidSysId := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId2.ToSystemID(),
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
			name: "System id in request and proof do not match",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1InvalidSysId, reqS2},
			},
			args: args{
				tb: trustBase,
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid partition 00000001 proof, node 1 request system id 00000002 does not match request",
		},
		{
			name: "IR change request, signature verification error",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{invalidReq},
			},
			args: args{
				tb: trustBase,
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "signature verification failed",
		},
		{
			name: "Proof contains more request, than there are partition nodes",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "proof contains more requests than registered partition nodes",
		},
		{
			name: "Proof contains more request from unknown node",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "3": v2},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "request proof from system id 00000001 node 2 is not valid: verification failed, unknown node id 2",
		},
		{
			name: "Proof contains duplicate node",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid proof, partition 00000001 proof contains duplicate request from node 1",
		},
		{
			name: "IR does not extend last certified state",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: []byte{0, 0, 1}, RoundNumber: 1},
				},
			},
			wantErrStr: "input record does not extend last certified",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReq{
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
		SystemIdentifier types.SystemID32
		CertReason       IRChangeReason
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		tb         partitions.PartitionTrustBase
		luc        *types.UnicityCertificate
		t2InRounds uint64
		round      uint64
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
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
				CertReason:       T2Timeout,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
					UnicitySeal: &types.UnicitySeal{
						RootChainRoundNumber: 1,
					},
				},
			},
			wantErrStr: "invalid partition 00000001 timeout proof, proof contains requests",
		},
		{
			name: "Invalid, not yet timeout",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       T2Timeout,
				Requests:         nil,
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash},
					UnicitySeal: &types.UnicitySeal{
						RootChainRoundNumber: 1,
					},
				},
				t2InRounds: 5,
				round:      5,
			},
			wantErrStr: "invalid partition 00000001 timeout proof, time from latest UC 4, timeout in rounds 5",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       T2Timeout,
				Requests:         nil,
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash},
					UnicitySeal: &types.UnicitySeal{
						RootChainRoundNumber: 1,
					},
				},
				t2InRounds: 5,
				round:      6,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReq{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			ir, err := x.Verify(tt.args.tb, tt.args.luc, tt.args.round, tt.args.t2InRounds)
			if tt.wantErrStr == "" {
				require.NoError(t, err)
				repeatIR := tt.args.luc.InputRecord.NewRepeatIR()
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
		SystemIdentifier types.SystemID32
		CertReason       IRChangeReason
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		tb  partitions.PartitionTrustBase
		luc *types.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)

	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
		NodeIdentifier:   "1",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS1.Sign(s1))
	reqS2 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
		NodeIdentifier:   "2",
		InputRecord:      inputRecord1,
	}
	require.NoError(t, reqS2.Sign(s2))
	reqS3NotMatchingIR := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
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
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "not enough requests to prove quorum",
		},
		{
			name: "IR does not extend last certified state",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: []byte{0, 0, 1}, RoundNumber: 1},
				},
			},
			wantErrStr: "input record does not extend last certified",
		},
		{
			name: "Contains not matching proofs",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2, reqS3NotMatchingIR},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid partition 00000001 quorum proof, contains proofs for different state hashes",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       Quorum,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReq{
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
		SystemIdentifier types.SystemID32
		CertReason       IRChangeReason
		Requests         []*certification.BlockCertificationRequest
	}
	type args struct {
		tb  partitions.PartitionTrustBase
		luc *types.UnicityCertificate
	}
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)

	reqS1 := &certification.BlockCertificationRequest{
		SystemIdentifier: sysId1.ToSystemID(),
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
				CertReason:       QuorumNotPossible,
				Requests:         []*certification.BlockCertificationRequest{reqS1},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
			wantErrStr: "invalid partition 00000001 no quorum proof, not enough requests to prove only no quorum is possible",
		},
		{
			name: "OK",
			fields: fields{
				SystemIdentifier: sysId1,
				CertReason:       QuorumNotPossible,
				Requests:         []*certification.BlockCertificationRequest{reqS1, reqS2},
			},
			args: args{
				tb: &partitions.TrustBase{
					PartitionTrustBase: map[string]crypto.Verifier{"1": v1, "2": v2},
				},
				luc: &types.UnicityCertificate{
					InputRecord: &types.InputRecord{Hash: prevHash, RoundNumber: 1},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReq{
				SystemIdentifier: tt.fields.SystemIdentifier,
				CertReason:       tt.fields.CertReason,
				Requests:         tt.fields.Requests,
			}
			ir, err := x.Verify(tt.args.tb, tt.args.luc, 0, 0)
			if tt.wantErrStr == "" {
				require.NoError(t, err)
				repeatIR := tt.args.luc.InputRecord.NewRepeatIR()
				require.Equal(t, repeatIR, ir)
				return
			}
			require.ErrorContains(t, err, tt.wantErrStr)
			require.Nil(t, ir)
		})
	}
}

func TestIRChangeReason_String(t *testing.T) {
	r := Quorum
	require.Equal(t, "quorum", r.String())
	require.Equal(t, "timeout", T2Timeout.String())
	require.Equal(t, "no-quorum", QuorumNotPossible.String())
	require.Equal(t, "unknown IR change reason 10", IRChangeReason(10).String())
}

func TestIRChangeReq_String(t *testing.T) {
	t.Run("stringer", func(t *testing.T) {
		x := &IRChangeReq{
			SystemIdentifier: sysId2,
			CertReason:       T2Timeout,
		}
		require.Equal(t, "00000002->timeout", x.String())
	})
}
