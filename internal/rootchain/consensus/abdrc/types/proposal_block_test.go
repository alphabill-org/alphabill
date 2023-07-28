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
	// returns valid BlockData obj - each test creates copy of it to
	// make single field/condition invalid
	validBlockData := func() *BlockData {
		return &BlockData{
			Author:    "author",
			Round:     8,
			Epoch:     0,
			Timestamp: 123456789000,
			Payload:   &Payload{}, // empty payload is OK
			Qc: &QuorumCert{
				VoteInfo:         &RoundInfo{ParentRoundNumber: 6, RoundNumber: 7, Timestamp: 1111, CurrentRootHash: []byte{0, 1, 3}},
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: []byte{0, 2, 1}},
				Signatures:       map[string][]byte{"1": {0, 1, 2}},
			},
		}
	}
	require.NoError(t, validBlockData().IsValid(), "validBlockData must return valid BlockData struct")

	t.Run("block round unassigned", func(t *testing.T) {
		bd := validBlockData()
		bd.Round = 0
		require.ErrorIs(t, bd.IsValid(), errRoundNumberUnassigned)
	})

	t.Run("block round must be greater than QC round", func(t *testing.T) {
		bd := validBlockData()
		bd.Round = bd.Qc.GetRound()
		err := bd.IsValid()
		require.EqualError(t, err, `invalid block round 7, round is less or equal to QC round 7`)

		bd.Round = bd.Qc.GetRound() - 1
		err = bd.IsValid()
		require.EqualError(t, err, `invalid block round 6, round is less or equal to QC round 7`)
	})

	t.Run("missing payload", func(t *testing.T) {
		bd := validBlockData()
		bd.Payload = nil
		require.ErrorIs(t, bd.IsValid(), errMissingPayload)
	})

	t.Run("invalid payload", func(t *testing.T) {
		bd := validBlockData()
		bd.Payload.Requests = []*IRChangeReq{{SystemIdentifier: []byte{1}}}
		err := bd.IsValid()
		require.EqualError(t, err, `invalid payload: invalid IR change request for 01: invalid system identifier [1]`)
	})

	t.Run("missing QC", func(t *testing.T) {
		bd := validBlockData()
		bd.Qc = nil
		require.ErrorIs(t, bd.IsValid(), errMissingQuorumCertificate)
	})

	t.Run("invalid QC", func(t *testing.T) {
		bd := validBlockData()
		bd.Qc.VoteInfo = nil
		require.ErrorIs(t, bd.IsValid(), errVoteInfoIsNil) // invalid quorum certificate: vote info is nil
	})
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
	require.ErrorContains(t, block.Verify(3, rootTrust), "invalid block data QC: quorum requires 3 signatures but certificate has 2")
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
