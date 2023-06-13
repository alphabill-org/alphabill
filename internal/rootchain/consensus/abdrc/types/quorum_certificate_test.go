package types

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

type Option func(*RoundInfo)

func NewDummyVoteInfo(round uint64, options ...Option) *RoundInfo {
	voteInfo := &RoundInfo{RoundNumber: round, Epoch: 0,
		Timestamp: 1670314583523, ParentRoundNumber: round - 1, CurrentRootHash: []byte{0, 1, 3}}
	for _, o := range options {
		o(voteInfo)
	}
	return voteInfo
}

func TestNewQuorumCertificate(t *testing.T) {
	voteInfo := &RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 3}}
	commitInfo := &CommitInfo{RootRoundInfoHash: []byte{0, 1, 2}}
	qc := NewQuorumCertificateFromVote(voteInfo, commitInfo, nil)
	require.NotNil(t, qc)
}

func TestQuorumCert_IsValid(t *testing.T) {
	type fields struct {
		VoteInfo         *RoundInfo
		LedgerCommitInfo *CommitInfo
		Signatures       map[string][]byte
	}
	voteInfo := &RoundInfo{RoundNumber: 10, Epoch: 0, Timestamp: 1670314583523, ParentRoundNumber: 9, CurrentRootHash: test.RandomBytes(32)}
	tests := []struct {
		name       string
		fields     fields
		wantErrStr string
	}{
		{
			name: "QC not  valid - no vote info",
			fields: fields{
				VoteInfo:         nil,
				LedgerCommitInfo: &CommitInfo{RootRoundInfoHash: voteInfo.Hash(gocrypto.SHA256)},
				Signatures:       nil,
			},
			wantErrStr: errVoteInfoIsNil.Error(),
		},
		// Vote info valid unit tests are covered in VoteInfo tests
		{
			name: "QC not  valid - vote info no valid",
			fields: fields{
				VoteInfo:         &RoundInfo{RoundNumber: 2, Epoch: 0, Timestamp: 0},
				LedgerCommitInfo: nil,
				Signatures:       nil,
			},
			wantErrStr: "vote info not valid, root round info latest root hash is not valid",
		},
		{
			name: "QC not  valid - ledger info is nil",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: nil,
				Signatures:       nil,
			},
			wantErrStr: errLedgerCommitInfoIsNil.Error(),
		},
		{
			name: "QC not  valid - ledger info is nil",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &CommitInfo{RootRoundInfoHash: nil, RootHash: nil},
				Signatures:       nil,
			},
			wantErrStr: errInvalidRoundInfoHash.Error(),
		},
		{
			name: "QC not  valid - missing signatures",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &CommitInfo{RootRoundInfoHash: voteInfo.Hash(gocrypto.SHA256)},
				Signatures:       nil,
			},
			wantErrStr: errQcIsMissingSignatures.Error(),
		},
		// Is valid, but would not verify
		{
			name: "QC valid - would not pass verify",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &CommitInfo{RootRoundInfoHash: voteInfo.Hash(gocrypto.SHA256)},
				Signatures: map[string][]byte{
					"test1": {0, 1, 2},
					"test2": {1, 2, 3},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &QuorumCert{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				Signatures:       tt.fields.Signatures,
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

func TestQuorumCert_Verify(t *testing.T) {
	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]crypto.Verifier{"1": v1, "2": v2, "3": v3}
	voteInfo := &RoundInfo{RoundNumber: 10, Epoch: 0,
		Timestamp: 1670314583523, ParentRoundNumber: 9, CurrentRootHash: test.RandomBytes(32)}
	commitInfo := &CommitInfo{RootRoundInfoHash: voteInfo.Hash(gocrypto.SHA256)}
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)

	type fields struct {
		VoteInfo         *RoundInfo
		LedgerCommitInfo *CommitInfo
		Signatures       map[string][]byte
	}
	type args struct {
		quorum    uint32
		rootTrust map[string]crypto.Verifier
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name: "QC not  valid - missing signatures",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       nil,
			},
			wantErrStr: "qc is missing signatures",
		},
		{
			name: "QC signatures not valid",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": {0, 1, 2}, "2": {0, 1, 2}, "3": {0, 1, 2}},
			},
			args:       args{quorum: 2, rootTrust: rootTrust},
			wantErrStr: "signature is not valid: signature length is",
		},
		{
			name: "QC no quorum",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": {0, 1, 2}, "2": {0, 1, 2}},
			},
			args:       args{quorum: 3, rootTrust: rootTrust},
			wantErrStr: "certificate has less signatures 2 than required by quorum 3",
		},
		{
			name: "QC invalid signature means no qc is not valid",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": {0, 1, 2}},
			},
			args:       args{quorum: 2, rootTrust: rootTrust},
			wantErrStr: "node 3 signature is not valid: signature length is",
		},
		{
			name: "QC is valid",
			fields: fields{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: commitInfo,
				Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
			},
			args: args{quorum: 2, rootTrust: rootTrust},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &QuorumCert{
				VoteInfo:         tt.fields.VoteInfo,
				LedgerCommitInfo: tt.fields.LedgerCommitInfo,
				Signatures:       tt.fields.Signatures,
			}
			err = x.Verify(tt.args.quorum, tt.args.rootTrust)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestQuorumCert_GetRound(t *testing.T) {
	var qc *QuorumCert = nil
	require.Equal(t, uint64(0), qc.GetRound())
	qc = &QuorumCert{}
	require.Equal(t, uint64(0), qc.GetRound())
	qc = &QuorumCert{VoteInfo: &RoundInfo{RoundNumber: 2}}
	require.Equal(t, uint64(2), qc.GetRound())
}

func TestQuorumCert_GetParentRound(t *testing.T) {
	var qc *QuorumCert = nil
	require.Equal(t, uint64(0), qc.GetParentRound())
	qc = &QuorumCert{}
	require.Equal(t, uint64(0), qc.GetParentRound())
	qc = &QuorumCert{VoteInfo: &RoundInfo{ParentRoundNumber: 2}}
	require.Equal(t, uint64(2), qc.GetParentRound())
}
