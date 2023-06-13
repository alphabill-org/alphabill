package types

import (
	"crypto"
	"crypto/sha256"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

func addSignature(t *testing.T, cert *QuorumCert, author string, signer abcrypto.Signer) {
	t.Helper()
	sig, err := signer.SignBytes(cert.LedgerCommitInfo.Bytes())
	require.NoError(t, err)
	cert.Signatures[author] = sig
}

func TestBlockDataHash(t *testing.T) {
	block := &BlockData{
		Author:    "test",
		Round:     2,
		Epoch:     0,
		Timestamp: 0x0102030405060708,
		Payload:   &Payload{},
		Qc: &QuorumCert{
			VoteInfo: &RoundInfo{
				RoundNumber:       1,
				Epoch:             0,
				Timestamp:         0x0010670314583523,
				ParentRoundNumber: 0,
				CurrentRootHash:   []byte{0, 1, 3}},
			LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 1, 2}, Hash: []byte{1, 2, 3}},
			Signatures:       map[string][]byte{"1": {1, 2, 3}, "2": {1, 2, 4}, "3": {1, 2, 5}},
		},
	}
	serializedBlock := []byte{
		't', 'e', 's', 't',
		0, 0, 0, 0, 0, 0, 0, 2,
		0, 0, 0, 0, 0, 0, 0, 0,
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		// Empty payload
		1, 2, 3, // "1" QC signature (in alphabetical order)
		1, 2, 4, // "2" QC signature
		1, 2, 5, // "3" QC signature
	}
	expected := sha256.Sum256(serializedBlock)
	hash := block.Hash(crypto.SHA256)
	require.NotNil(t, hash)
	require.Equal(t, expected[:], hash)
}

func TestBlockData_IsValid(t *testing.T) {
	type fields struct {
		Id        []byte
		Author    string
		Round     uint64
		Epoch     uint64
		Timestamp uint64
		Payload   *Payload
		Qc        *QuorumCert
	}
	type args struct {
		quorum    uint32
		rootTrust map[string]abcrypto.Verifier
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Invalid block round",
			fields: fields{
				Id:        nil,
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &RoundInfo{RoundNumber: 0, CurrentRootHash: []byte{0, 1, 3}},
					LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
				},
			},
			args:       args{quorum: 1, rootTrust: nil},
			wantErrStr: "root round info round number is not valid",
		},
		{
			name: "Invalid round number",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     0,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 3}},
					LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
				},
			},
			args:       args{quorum: 1, rootTrust: nil},
			wantErrStr: errInvalidRound.Error(),
		},
		{
			name: "Invalid payload",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   nil, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 3}},
					LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
				},
			},
			args:       args{quorum: 1, rootTrust: nil},
			wantErrStr: errMissingPayload.Error(),
		},
		{
			name: "Invalid QC is nil",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc:        nil,
			},
			args:       args{quorum: 1, rootTrust: nil},
			wantErrStr: errMissingQuorumCertificate.Error(),
		},
		{
			name: "Invalid timestamp missing",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 3}},
					LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
				},
			},
			args:       args{quorum: 1, rootTrust: nil},
			wantErrStr: "round creation time not set",
		},
		{
			name: "Invalid timestamp missing",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 3}},
					LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
				},
			},
			args:       args{quorum: 1, rootTrust: nil},
			wantErrStr: "round creation time not set",
		},
		{
			name: "Invalid block round, Qc round is higher",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &RoundInfo{RoundNumber: 3, Timestamp: 1111, CurrentRootHash: []byte{0, 1, 3}},
					LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
					Signatures:       map[string][]byte{"1": {0, 1, 2}},
				},
			},
			args:       args{quorum: 1, rootTrust: nil},
			wantErrStr: "invalid block round 2, round is less or equal to qc round 3",
		},
		{
			name: "Valid",
			fields: fields{
				Id:        []byte{0, 1, 2},
				Author:    "test",
				Round:     2,
				Epoch:     0,
				Timestamp: 0x0102030405060708,
				Payload:   &Payload{}, // empty payload
				Qc: &QuorumCert{
					VoteInfo:         &RoundInfo{RoundNumber: 1, Timestamp: 1111, CurrentRootHash: []byte{0, 1, 3}},
					LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
					Signatures:       map[string][]byte{"1": {0, 1, 2}},
				},
			},
			args: args{quorum: 1, rootTrust: nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &BlockData{
				Author:    tt.fields.Author,
				Round:     tt.fields.Round,
				Epoch:     tt.fields.Epoch,
				Timestamp: tt.fields.Timestamp,
				Payload:   tt.fields.Payload,
				Qc:        tt.fields.Qc,
			}
			err := x.IsValid()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestBlockData_Verify(t *testing.T) {
	// not valid case
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]abcrypto.Verifier{"1": v1, "2": v2, "3": v3}
	info := NewDummyVoteInfo(1)
	block := &BlockData{
		Author:    "test",
		Round:     2,
		Epoch:     0,
		Timestamp: 0x0102030405060708,
		Payload:   &Payload{}, // empty payload
		Qc:        NewQuorumCertificate(info, nil),
	}
	require.ErrorContains(t, block.Verify(3, rootTrust), "qc is missing signatures")
	addSignature(t, block.Qc, "1", s1)
	addSignature(t, block.Qc, "2", s2)
	addSignature(t, block.Qc, "3", s3)
	require.NoError(t, block.Verify(3, rootTrust))
	// remove a signature from QC
	delete(block.Qc.Signatures, "2")
	require.ErrorContains(t, block.Verify(3, rootTrust), "block data quorum certificate validation failed, certificate has less signatures 2 than required by quorum 3")
}

func TestPayload_IsEmpty(t *testing.T) {
	type fields struct {
		Requests []*IRChangeReq
	}
	tests := []struct {
		name   string
		fields fields
		want   bool
	}{
		{
			name:   "empty (nil)",
			fields: fields{Requests: nil},
			want:   true,
		},
		{
			name:   "empty",
			fields: fields{Requests: []*IRChangeReq{}},
			want:   true,
		},
		{
			name: "not empty",
			fields: fields{Requests: []*IRChangeReq{
				{SystemIdentifier: []byte{0, 0, 0, 1}, CertReason: T2Timeout},
			}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &Payload{
				Requests: tt.fields.Requests,
			}
			if got := x.IsEmpty(); got != tt.want {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPayload_IsValid(t *testing.T) {
	type fields struct {
		Requests []*IRChangeReq
	}
	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name:       "empty (nil)",
			fields:     fields{Requests: nil},
			wantErrStr: "",
		},
		{
			name:       "empty",
			fields:     fields{Requests: []*IRChangeReq{}},
			wantErrStr: "",
		},
		{
			name: "valid timeout",
			fields: fields{Requests: []*IRChangeReq{
				{SystemIdentifier: []byte{0, 0, 0, 1}, CertReason: T2Timeout},
			}},
			wantErrStr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &Payload{
				Requests: tt.fields.Requests,
			}
			err := x.IsValid()
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestBlockData_GetParentRound(t *testing.T) {
	var x *BlockData = nil
	require.Equal(t, uint64(0), x.GetParentRound())
	x = &BlockData{}
	require.Equal(t, uint64(0), x.GetParentRound())
	x = &BlockData{Qc: &QuorumCert{}}
	require.Equal(t, uint64(0), x.GetParentRound())
	x = &BlockData{Qc: &QuorumCert{VoteInfo: NewDummyVoteInfo(1)}}
	require.Equal(t, uint64(1), x.GetParentRound())
}
