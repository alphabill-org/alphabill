package types

import (
	"bytes"
	gocrypto "crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
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
	voteInfo := &RoundInfo{
		RoundNumber:       8,
		ParentRoundNumber: 7,
		Epoch:             0,
		Timestamp:         1670314583523,
		CurrentRootHash:   test.RandomBytes(32)}
	timeoutCert := &TimeoutCert{
		Timeout: &Timeout{
			Epoch: 0,
			Round: 10,
			HighQc: &QuorumCert{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
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
			LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	err := timeoutCert.Add("1", t1, []byte{0, 1, 2})
	require.NoError(t, err)
	require.Equal(t, []string{"1"}, timeoutCert.GetAuthors())
	require.Equal(t, timeoutCert.Timeout.HighQc.VoteInfo.RoundNumber, voteInfo.RoundNumber)
	// Add a new timeout vote, but with lower round
	voteInfo = &RoundInfo{
		RoundNumber:       7,
		ParentRoundNumber: 6,
		Epoch:             0,
		Timestamp:         1670314583523,
		CurrentRootHash:   test.RandomBytes(32)}
	t2 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	err = timeoutCert.Add("2", t2, []byte{1, 2, 2})
	require.NoError(t, err)
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Equal(t, uint64(8), timeoutCert.Timeout.HighQc.VoteInfo.RoundNumber)
	// Add a third vote, but with higher QC round so QC in the certificate gets updated
	voteInfo = &RoundInfo{
		RoundNumber:       9,
		ParentRoundNumber: 8,
		Epoch:             0,
		Timestamp:         1670314583523,
		CurrentRootHash:   test.RandomBytes(32)}
	t3 := &Timeout{
		Epoch: 0,
		Round: 10,
		HighQc: &QuorumCert{
			VoteInfo:         voteInfo,
			LedgerCommitInfo: &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)},
			Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
		},
	}
	err = timeoutCert.Add("3", t3, []byte{1, 2, 2})
	require.NoError(t, err)
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Contains(t, timeoutCert.GetAuthors(), "3")
	require.Equal(t, uint64(9), timeoutCert.Timeout.HighQc.VoteInfo.RoundNumber)
	// duplicate
	err = timeoutCert.Add("3", t3, []byte{1, 2, 3})
	require.ErrorContains(t, err, "already voted")
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
	voteInfo := &RoundInfo{
		RoundNumber:       timeoutRound - 1,
		ParentRoundNumber: timeoutRound - 2,
		Epoch:             0,
		Timestamp:         1670314583523,
		CurrentRootHash:   test.RandomBytes(32)}
	commitInfo := &types.UnicitySeal{RootInternalInfo: voteInfo.Hash(gocrypto.SHA256)}
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
			if err = x.Verify(tt.args.quorum, tt.args.rootTrust); (err != nil) != tt.wantErr {
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
			VoteInfo: &RoundInfo{
				RoundNumber:       7,
				Epoch:             0,
				ParentRoundNumber: 6,
			},
		},
	}
	rootTrust := map[string]abcrypto.Verifier{}
	err := timeout.Verify(2, rootTrust)
	require.ErrorContains(t, err, "invalid timeout, qc round 7 is bigger than timeout round 3")
	// QC verification is unit-tested in QC module
}

func TestTimeoutCert_GetRound(t *testing.T) {
	var tc *TimeoutCert = nil
	require.Equal(t, uint64(0), tc.GetRound())
	tc = &TimeoutCert{Timeout: nil}
	require.Equal(t, uint64(0), tc.GetRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{Round: 10},
	}
	require.Equal(t, uint64(10), tc.GetRound())
}

func TestTimeoutCert_GetHqcRound(t *testing.T) {
	var tc *TimeoutCert = nil
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{Timeout: nil}
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{
			Round:  10,
			HighQc: nil,
		},
	}
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{
			Round:  10,
			HighQc: &QuorumCert{},
		},
	}
	require.Equal(t, uint64(0), tc.GetHqcRound())
	tc = &TimeoutCert{
		Timeout: &Timeout{
			Round:  10,
			HighQc: &QuorumCert{VoteInfo: &RoundInfo{RoundNumber: 9}},
		},
	}
	require.Equal(t, uint64(9), tc.GetHqcRound())
}

func TestBytesFromTimeoutVote(t *testing.T) {
	timeout := &Timeout{
		Round: 10,
		Epoch: 0,
		HighQc: &QuorumCert{
			VoteInfo: &RoundInfo{
				RoundNumber:       9,
				Epoch:             0,
				Timestamp:         0x0010670314583523,
				ParentRoundNumber: 8,
				CurrentRootHash:   []byte{0, 1, 3}},
		},
	}
	// Require serialization is equal
	b := BytesFromTimeoutVote(timeout, "test", &TimeoutVote{HqcRound: 9, Signature: []byte{1, 2, 3}})
	require.NotNil(t, b)
	require.Len(t, b, 28)
}
