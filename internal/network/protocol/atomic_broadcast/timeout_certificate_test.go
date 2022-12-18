package atomic_broadcast

import (
	"bytes"
	"crypto"
	"github.com/alphabill-org/alphabill/internal/util"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func calcTimeoutSig(t *testing.T, s abcrypto.Signer, round, epoch, hQcRound uint64, author string) []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(round))
	b.Write(util.Uint64ToBytes(epoch))
	b.Write(util.Uint64ToBytes(hQcRound))
	b.Write([]byte(author))
	sig, err := s.SignBytes(b.Bytes())
	require.NoError(t, err)
	return sig
}

func TestTimeoutCert_Add(t *testing.T) {
	// create partial timeout certificate
	voteInfo := NewDummyVoteInfo(8)
	timeoutCert := &TimeoutCert{
		Timeout: &Timeout{
			Epoch: 0,
			Round: 10,
			HighQc: &QuorumCert{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, voteInfo),
				Signatures:       map[string][]byte{"1": {1, 2, 1}},
			},
		},
		Signatures: make(map[string]*TimeoutVote),
	}
	t1 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, voteInfo),
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	timeoutCert.Add("1", t1, []byte{0, 1, 2})
	require.Equal(t, []string{"1"}, timeoutCert.GetAuthors())
	require.Equal(t, timeoutCert.Timeout.HighQc.VoteInfo.RootRound, voteInfo.RootRound)
	// Add a new timeout vote, but with lower round
	t2 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         NewDummyVoteInfo(7),
			LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, NewDummyVoteInfo(7)),
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	timeoutCert.Add("2", t2, []byte{1, 2, 2})
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Equal(t, uint64(8), timeoutCert.Timeout.HighQc.VoteInfo.RootRound)
	// Add a third vote, but with higher QC round so QC in the certificate gets updated
	t3 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         NewDummyVoteInfo(9),
			LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, NewDummyVoteInfo(9)),
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	timeoutCert.Add("3", t3, []byte{1, 2, 2})
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Contains(t, timeoutCert.GetAuthors(), "3")
	require.Equal(t, uint64(9), timeoutCert.Timeout.HighQc.VoteInfo.RootRound)
}

func TestTimeoutCert_Verify(t *testing.T) {
	type fields struct {
		Timeout    *Timeout
		Signatures map[string]*TimeoutVote
	}
	type args struct {
		quorum    uint32
		rootTrust map[string]abcrypto.Verifier
	}
	const timeoutRound = 9

	s1, v1 := testsig.CreateSignerAndVerifier(t)
	s2, v2 := testsig.CreateSignerAndVerifier(t)
	s3, v3 := testsig.CreateSignerAndVerifier(t)
	rootTrust := map[string]abcrypto.Verifier{"1": v1, "2": v2, "3": v3}
	voteInfo := NewDummyVoteInfo(timeoutRound - 1)
	commitInfo := NewDummyCommitInfo(crypto.SHA256, voteInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)

	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "valid TC",
			fields: fields{
				Timeout: &Timeout{
					Epoch: 0,
					Round: 9,
					HighQc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-2, "1")},
					"2": {HqcRound: timeoutRound - 1, Signature: calcTimeoutSig(t, s2, timeoutRound, 0, timeoutRound-1, "2")},
					"3": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s3, timeoutRound, 0, timeoutRound-2, "3")},
				},
			},
			args: args{
				quorum:    3,
				rootTrust: rootTrust,
			},
		},
		{
			name: "no quorum",
			fields: fields{
				Timeout: &Timeout{
					Epoch: 0,
					Round: 9,
					HighQc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-2, "1")},
					"2": {HqcRound: timeoutRound - 1, Signature: calcTimeoutSig(t, s2, timeoutRound, 0, timeoutRound-1, "2")},
				},
			},
			args: args{
				quorum:    3,
				rootTrust: rootTrust,
			},
			wantErr: true,
		},
		{
			name: "Invalid QC on timeout certificate",
			fields: fields{
				Timeout: &Timeout{
					Epoch: 0,
					Round: 9,
					HighQc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": {0, 1, 2}},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-2, "1")},
					"2": {HqcRound: timeoutRound - 1, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-1, "2")},
					"3": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s3, timeoutRound, 0, timeoutRound-2, "3")},
				},
			},
			args: args{
				quorum:    3,
				rootTrust: rootTrust,
			},
			wantErr: true,
		},
		{
			name: "Hqc and signature qc rounds do not match",
			fields: fields{
				Timeout: &Timeout{
					Epoch: 0,
					Round: 9,
					HighQc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-2, "1")},
					"2": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-2, "2")},
					"3": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s3, timeoutRound, 0, timeoutRound-2, "3")},
				},
			},
			args: args{
				quorum:    3,
				rootTrust: rootTrust,
			},
			wantErr: true,
		},
		{
			name: "Invalid signature, hqc round ",
			fields: fields{
				Timeout: &Timeout{
					Epoch: 0,
					Round: 9,
					HighQc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-2, "1")},
					"2": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s1, timeoutRound, 0, timeoutRound-1, "2")},
					"3": {HqcRound: timeoutRound - 2, Signature: calcTimeoutSig(t, s3, timeoutRound, 0, timeoutRound-2, "3")},
				},
			},
			args: args{
				quorum:    3,
				rootTrust: rootTrust,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &TimeoutCert{
				Timeout:    tt.fields.Timeout,
				Signatures: tt.fields.Signatures,
			}
			if err := x.Verify(tt.args.quorum, tt.args.rootTrust); (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTimeout_Verify(t *testing.T) {
	timeout := &Timeout{
		Epoch: 0,
		Round: 3,
		HighQc: &QuorumCert{
			VoteInfo: &VoteInfo{
				RootRound:   7,
				Epoch:       0,
				ParentRound: 6,
			},
		},
	}
	rootTrust := map[string]abcrypto.Verifier{}
	err := timeout.Verify(2, rootTrust)
	require.ErrorContains(t, err, "invalid timeout, qc round 7 is bigger than timeout round 3")
	// QC verification is unit-tested in QC module
}
