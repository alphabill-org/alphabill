package types

import (
	"crypto"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/types"
)

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
			LedgerCommitInfo: &types.UnicitySeal{PreviousHash: []byte{0, 1, 2}, Hash: []byte{1, 2, 3}},
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
				LedgerCommitInfo: &types.UnicitySeal{PreviousHash: []byte{0, 2, 1}},
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
	sb := newStructBuilder(t, 3)
	rootTrust := sb.Verifiers()

	t.Run("IsValid is called", func(t *testing.T) {
		bd := sb.BlockData(t)
		bd.Round = 0
		require.ErrorIs(t, bd.Verify(3, rootTrust), errRoundNumberUnassigned)
	})

	t.Run("no quorum for QC", func(t *testing.T) {
		// basically check that QC.Verify is called
		bd := sb.BlockData(t)
		err := bd.Verify(uint32(len(bd.Qc.Signatures)+1), rootTrust)
		require.EqualError(t, err, `invalid block data QC: quorum requires 4 signatures but certificate has 3`)
	})
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
