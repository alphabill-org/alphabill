package atomic_broadcast

import (
	gocrypto "crypto"
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestTimeout_Bytes(t *testing.T) {
	type fields struct {
		Epoch uint64
		Round uint64
		Hqc   *QuorumCert
	}
	tests := []struct {
		name   string
		fields fields
		want   []byte
	}{
		{
			name: "verify serialization",
			fields: fields{
				Epoch: 1,
				Round: 2,
				Hqc: &QuorumCert{
					VoteInfo:         NewDummyVoteInfo(10),
					LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 1, 2}, CommitStateId: []byte{2, 3, 4}},
					Signatures:       map[string][]byte{},
				},
			},
			// Timeout is serialized without QC, since the latter is signed anyway
			want: []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &Timeout{
				Epoch: tt.fields.Epoch,
				Round: tt.fields.Round,
				Hqc:   tt.fields.Hqc,
			}
			if got := x.Bytes(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Bytes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVoteMsg_AddSignature(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		HighCommitQc     *QuorumCert
		Author           string
	}
	type args struct {
		signer crypto.Signer
	}
	s1, _ := testsig.CreateSignerAndVerifier(t)
	voteInfo := NewDummyVoteInfo(10)
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Sign ok",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteInfo),
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "",
		},
		{
			name: "Vote info hash is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: s1},
			wantErrStr: "invalid vote info hash",
		},
		{
			name: "Signer is nil",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
			},
			args:       args{signer: nil},
			wantErrStr: "signer is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &VoteMsg{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				HighCommitQc:     tt.fields.HighCommitQc,
				Author:           tt.fields.Author,
			}
			err := x.AddSignature(tt.args.signer)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.False(t, x.IsTimeout())
			require.NoError(t, err)
		})
	}
}

func TestVoteMsg_AddTimeoutSignature(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		HighCommitQc     *QuorumCert
		Author           string
		Signature        []byte
		TimeoutSignature *TimeoutWithSignature
	}
	type args struct {
		timeout   *Timeout
		signature []byte
	}
	const timeoutRound = 10
	s1, _ := testsig.CreateSignerAndVerifier(t)
	// previous round qc
	voteInfo := NewDummyVoteInfo(timeoutRound - 1)
	prevRoundQc := &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteInfo),
		Signatures:       map[string][]byte{},
	}
	timeout, err := NewTimeout(timeoutRound, 0, prevRoundQc)
	require.NoError(t, err)
	sig, err := s1.SignBytes(timeout.Bytes())
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Pure timeout vote",
			fields: fields{
				VoteInfo:         NewDummyVoteInfo(timeoutRound),
				LedgerCommitInfo: nil,
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
			},
			args:       args{timeout: timeout, signature: sig},
			wantErrStr: "",
		},
		{
			name: "Missing timeout",
			fields: fields{
				VoteInfo:         NewDummyVoteInfo(timeoutRound),
				LedgerCommitInfo: nil,
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
			},
			args:       args{timeout: nil, signature: sig},
			wantErrStr: "timeout is nil",
		},
		{
			name: "Missing signatures",
			fields: fields{
				VoteInfo:         NewDummyVoteInfo(timeoutRound),
				LedgerCommitInfo: nil,
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
			},
			args:       args{timeout: timeout, signature: nil},
			wantErrStr: "error timeout is not signed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &VoteMsg{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				HighCommitQc:     tt.fields.HighCommitQc,
				Author:           tt.fields.Author,
				Signature:        tt.fields.Signature,
				TimeoutSignature: tt.fields.TimeoutSignature,
			}
			err := x.AddTimeoutSignature(tt.args.timeout, tt.args.signature)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
			require.True(t, x.IsTimeout())
		})
	}
}

func TestVoteMsg_NewTimeout(t *testing.T) {
	type args struct {
		round uint64
		epoch uint64
		qc    *QuorumCert
	}
	const timeoutRound = 10
	prevRoundInfo := NewDummyVoteInfo(timeoutRound - 1)
	tests := []struct {
		name       string
		args       args
		wantErrStr string
		want       *Timeout
	}{
		{
			name: "ok",
			args: args{
				round: timeoutRound,
				epoch: 0,
				qc: &QuorumCert{
					VoteInfo:         prevRoundInfo,
					LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, prevRoundInfo),
					Signatures:       map[string][]byte{},
				}},
		},
		{
			name: "round is in past",
			args: args{
				round: timeoutRound - 1,
				epoch: 0,
				qc: &QuorumCert{
					VoteInfo:         prevRoundInfo,
					LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, prevRoundInfo),
					Signatures:       map[string][]byte{},
				}},
			wantErrStr: "invalid timeout round",
		},
		{
			name: "high qc is nil",
			args: args{
				round: timeoutRound,
				epoch: 0,
				qc:    nil,
			},
			wantErrStr: "invalid timeout, high qc is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTimeout(tt.args.round, tt.args.epoch, tt.args.qc)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, got.Round, tt.args.round)
			require.Equal(t, got.Epoch, tt.args.epoch)
		})
	}
}

func TestVoteMsg_Verify(t *testing.T) {
	type fields struct {
		VoteInfo         *VoteInfo
		LedgerCommitInfo *LedgerCommitInfo
		HighCommitQc     *QuorumCert
		Author           string
		Signature        []byte
		TimeoutSignature *TimeoutWithSignature
	}
	type args struct {
		quorum    uint32
		rootTrust map[string]crypto.Verifier
	}
	const votedRound = 10
	const quorum = 3
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	commitQcInfo := NewDummyVoteInfo(votedRound - 2)
	commitInfo := NewDummyCommitInfo(gocrypto.SHA256, commitQcInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	commitQc := &QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	voteMsgInfo := NewDummyVoteInfo(votedRound)
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "Vote info error",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
				Signature:        nil,
				TimeoutSignature: nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, vote info is nil",
		},
		{
			name: "Vote info invalid",
			fields: fields{
				VoteInfo:         &VoteInfo{RootRound: 8, ParentRound: 9},
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: nil, CommitStateId: nil},
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
				Signature:        nil,
				TimeoutSignature: nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, invalid round number",
		},
		{
			name: "Vote info hash error",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: &LedgerCommitInfo{VoteInfoHash: []byte{0, 1, 3}, CommitStateId: nil},
				HighCommitQc:     &QuorumCert{},
				Author:           "test",
				Signature:        nil,
				TimeoutSignature: nil,
			},
			args:       args{quorum: quorum, rootTrust: nil},
			wantErrStr: "invalid vote message, vote info hash verification failed",
		},
		{
			name: "Vote info no author",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighCommitQc:     &QuorumCert{},
				Author:           "",
				Signature:        nil,
				TimeoutSignature: nil,
			},
			args:       args{quorum: quorum, rootTrust: rootTrust},
			wantErrStr: "invalid vote message, no author",
		},
		{
			name: "Vote info author public key not registered",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighCommitQc:     commitQc,
				Author:           "test",
				Signature:        nil,
				TimeoutSignature: nil,
			},
			args:       args{quorum: quorum, rootTrust: rootTrust},
			wantErrStr: "failed to find public key for author test",
		},
		{
			name: "Vote info author public key not registered",
			fields: fields{
				VoteInfo:         voteMsgInfo,
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
				HighCommitQc:     commitQc,
				Author:           "1",
				Signature:        nil,
				TimeoutSignature: nil,
			},
			args:       args{quorum: quorum, rootTrust: rootTrust},
			wantErrStr: "signature verification failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &VoteMsg{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				HighCommitQc:     tt.fields.HighCommitQc,
				Author:           tt.fields.Author,
				Signature:        tt.fields.Signature,
				TimeoutSignature: tt.fields.TimeoutSignature,
			}
			err := x.Verify(tt.args.quorum, tt.args.rootTrust)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestVoteMsg_VerifyOk(t *testing.T) {
	const votedRound = 10
	const quorum = 3
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	commitQcInfo := NewDummyVoteInfo(votedRound - 2)
	commitInfo := NewDummyCommitInfo(gocrypto.SHA256, commitQcInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	commitQc := &QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	voteMsgInfo := NewDummyVoteInfo(votedRound)
	voteMsg := &VoteMsg{
		VoteInfo:         voteMsgInfo,
		LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
		HighCommitQc:     commitQc,
		Author:           "1",
	}
	require.NoError(t, voteMsg.AddSignature(s1))
	require.False(t, voteMsg.IsTimeout())
	require.NoError(t, voteMsg.Verify(quorum, rootTrust))
	// Add timeout signature to make it a timeout vote
	timeout, err := NewTimeout(voteMsg.VoteInfo.RootRound, voteMsg.VoteInfo.Epoch, voteMsg.HighCommitQc)
	require.NoError(t, err)
	sig, err := s1.SignBytes(timeout.Bytes())
	require.NoError(t, err)
	require.NoError(t, voteMsg.AddTimeoutSignature(timeout, sig))
	require.NoError(t, voteMsg.Verify(quorum, rootTrust))
	require.True(t, voteMsg.IsTimeout())
}

func TestVoteMsg_PureTimeoutVoteVerifyOk(t *testing.T) {
	const votedRound = 10
	const quorum = 3
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	commitQcInfo := NewDummyVoteInfo(votedRound - 2)
	commitInfo := NewDummyCommitInfo(gocrypto.SHA256, commitQcInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	commitQc := &QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	voteMsgInfo := NewDummyVoteInfo(votedRound)
	voteMsg := &VoteMsg{
		VoteInfo:         voteMsgInfo,
		LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, voteMsgInfo),
		HighCommitQc:     commitQc,
		Author:           "1",
	}
	require.NoError(t, voteMsg.AddSignature(s1))
	require.False(t, voteMsg.IsTimeout())
	require.NoError(t, voteMsg.Verify(quorum, rootTrust))
	// Add timeout signature to make it a timeout vote
	timeout, err := NewTimeout(voteMsg.VoteInfo.RootRound, voteMsg.VoteInfo.Epoch, voteMsg.HighCommitQc)
	require.NoError(t, err)
	sig, err := s1.SignBytes(timeout.Bytes())
	require.NoError(t, err)
	require.NoError(t, voteMsg.AddTimeoutSignature(timeout, sig))
	require.NoError(t, voteMsg.Verify(quorum, rootTrust))
	require.True(t, voteMsg.IsTimeout())
}
