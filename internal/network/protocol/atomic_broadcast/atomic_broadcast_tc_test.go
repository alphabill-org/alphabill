package atomic_broadcast

import (
	"crypto"
	"crypto/sha256"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
)

func TestNewTimeoutSign(t *testing.T) {
	type args struct {
		epoch    uint64
		round    uint64
		hqcRound uint64
	}
	tests := []struct {
		name string
		args args
		want *TimeoutSigned
	}{
		{
			name: "All ok, simple test",
			args: args{epoch: 0, round: 1, hqcRound: 1},
			want: &TimeoutSigned{
				epoch:    0,
				round:    1,
				hqcRound: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewTimeoutSign(tt.args.epoch, tt.args.round, tt.args.hqcRound); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewTimeoutSign() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeoutCert_AddSignature(t *testing.T) {
	// create partial timeout certificate
	voteInfo := &VoteInfo{
		RootRound:   8,
		Epoch:       0,
		ParentRound: 7,
	}
	timeoutCert := &TimeoutCert{
		Timeout: &Timeout{
			Epoch: 0,
			Round: 10,
			Hqc: &QuorumCert{
				VoteInfo:         voteInfo,
				LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, dummyVoteInfo),
				Signatures:       map[string][]byte{"1": {1, 2, 1}},
			},
		},
		Signatures: make(map[string]*TimeoutVote),
	}
	t1 := &TimeoutWithSignature{
		Timeout: &Timeout{
			Epoch: 0,
			Round: 10,
			Hqc: &QuorumCert{
				VoteInfo: &VoteInfo{
					RootRound:   8,
					Epoch:       0,
					ParentRound: 7,
				},
				LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, dummyVoteInfo),
				Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
			},
		},
		Signature: []byte{1, 2, 2},
	}
	timeoutCert.AddSignature("1", t1)
	require.Equal(t, []string{"1"}, timeoutCert.GetAuthors())
	require.Equal(t, timeoutCert.Timeout.Hqc.VoteInfo.RootRound, voteInfo.RootRound)
	// Add a new timeout vote, but with lower round
	t2 := &TimeoutWithSignature{
		Timeout: &Timeout{
			Epoch: 0,
			Round: 10,
			Hqc: &QuorumCert{
				VoteInfo: &VoteInfo{
					RootRound:   7,
					Epoch:       0,
					ParentRound: 6,
				},
				LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, dummyVoteInfo),
				Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
			},
		},
		Signature: []byte{1, 2, 2},
	}
	timeoutCert.AddSignature("2", t2)
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Equal(t, uint64(8), timeoutCert.Timeout.Hqc.VoteInfo.RootRound)
	// Add a third vote, but with higher QC round so QC in the certificate gets updated
	t3 := &TimeoutWithSignature{
		Timeout: &Timeout{
			Epoch: 0,
			Round: 10,
			Hqc: &QuorumCert{
				VoteInfo: &VoteInfo{
					RootRound:   9,
					Epoch:       0,
					ParentRound: 8,
				},
				LedgerCommitInfo: NewDummyCommitInfo(crypto.SHA256, dummyVoteInfo),
				Signatures:       map[string][]byte{"1": {1, 2, 1}, "2": {1, 2, 3}, "3": {1, 2, 3}},
			},
		},
		Signature: []byte{1, 2, 2},
	}
	timeoutCert.AddSignature("3", t3)
	require.Contains(t, timeoutCert.GetAuthors(), "1")
	require.Contains(t, timeoutCert.GetAuthors(), "2")
	require.Contains(t, timeoutCert.GetAuthors(), "3")
	require.Equal(t, uint64(9), timeoutCert.Timeout.Hqc.VoteInfo.RootRound)
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
	voteInfo := &VoteInfo{
		BlockId:       []byte{0, 1, 1},
		RootRound:     timeoutRound - 1,
		Epoch:         0,
		Timestamp:     9,
		ParentBlockId: []byte{0, 1},
		ParentRound:   timeoutRound - 2,
		ExecStateId:   []byte{0, 1, 3},
	}
	commitInfo := NewDummyCommitInfo(crypto.SHA256, voteInfo)
	sig1, err := s1.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig2, err := s2.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	sig3, err := s3.SignBytes(commitInfo.Bytes())
	require.NoError(t, err)
	timeoutMsg1 := NewTimeoutSign(timeoutRound, 0, timeoutRound-2)
	tMsgSig1, err := s1.SignHash(timeoutMsg1.Hash(crypto.SHA256))
	require.NoError(t, err)
	timeoutMsg2 := NewTimeoutSign(timeoutRound, 0, timeoutRound-1)
	tMsgSig2, err := s2.SignHash(timeoutMsg2.Hash(crypto.SHA256))
	require.NoError(t, err)
	timeoutMsg2Rnd := NewTimeoutSign(timeoutRound, 0, timeoutRound-2)
	tMsgSig2Rnd, err := s2.SignHash(timeoutMsg2Rnd.Hash(crypto.SHA256))
	require.NoError(t, err)

	timeoutMsg3 := NewTimeoutSign(timeoutRound, 0, timeoutRound-2)
	tMsgSig3, err := s3.SignHash(timeoutMsg3.Hash(crypto.SHA256))
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
					Hqc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: tMsgSig1},
					"2": {HqcRound: timeoutRound - 1, Signature: tMsgSig2},
					"3": {HqcRound: timeoutRound - 2, Signature: tMsgSig3},
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
					Hqc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: tMsgSig1},
					"2": {HqcRound: timeoutRound - 1, Signature: tMsgSig2},
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
					Hqc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": {0, 1, 2}},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: tMsgSig1},
					"2": {HqcRound: timeoutRound - 2, Signature: tMsgSig2Rnd},
					"3": {HqcRound: timeoutRound - 2, Signature: tMsgSig3},
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
					Hqc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: tMsgSig1},
					"2": {HqcRound: timeoutRound - 2, Signature: tMsgSig2},
					"3": {HqcRound: timeoutRound - 2, Signature: tMsgSig3},
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
					Hqc: &QuorumCert{
						VoteInfo:         voteInfo,
						LedgerCommitInfo: commitInfo,
						Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
					},
				},
				Signatures: map[string]*TimeoutVote{
					"1": {HqcRound: timeoutRound - 2, Signature: tMsgSig1},
					"2": {HqcRound: timeoutRound - 2, Signature: tMsgSig2},
					"3": {HqcRound: timeoutRound - 2, Signature: tMsgSig3},
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

func TestTimeoutSigned_Hash(t1 *testing.T) {
	type fields struct {
		epoch    uint64
		round    uint64
		hqcRound uint64
	}
	type args struct {
		algo crypto.Hash
	}
	hash := sha256.Sum256([]byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 2})
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []byte
	}{
		{
			name:   "verify all fields included",
			fields: fields{epoch: 1, round: 3, hqcRound: 2},
			args:   args{algo: crypto.SHA256},
			want:   hash[:],
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &TimeoutSigned{
				epoch:    tt.fields.epoch,
				round:    tt.fields.round,
				hqcRound: tt.fields.hqcRound,
			}
			if got := t.Hash(tt.args.algo); !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("Hash() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeout_Verify(t *testing.T) {
	timeout := &Timeout{
		Epoch: 0,
		Round: 3,
		Hqc: &QuorumCert{
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
