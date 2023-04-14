package ab_consensus

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestTimeoutMsg_Bytes(t *testing.T) {
	timeoutMsg := &TimeoutMsg{
		Timeout: &Timeout{
			Round: 10,
			Epoch: 0,
			HighQc: &QuorumCert{
				VoteInfo: &certificates.RootRoundInfo{
					RoundNumber:       9,
					Epoch:             0,
					Timestamp:         0x0010670314583523,
					ParentRoundNumber: 8,
					CurrentRootHash:   []byte{0, 1, 3}},
			},
		},
		Author:    "test",
		Signature: []byte{1, 2, 3},
	}
	serialized := []byte{
		0, 0, 0, 0, 0, 0, 0, 10,
		0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 9,
		't', 'e', 's', 't',
	}
	require.Equal(t, serialized, timeoutMsg.Bytes())
}

func TestBytesFromTimeoutVote(t *testing.T) {
	timeoutMsg := &TimeoutMsg{
		Timeout: &Timeout{
			Round: 10,
			Epoch: 0,
			HighQc: &QuorumCert{
				VoteInfo: &certificates.RootRoundInfo{
					RoundNumber:       9,
					Epoch:             0,
					Timestamp:         0x0010670314583523,
					ParentRoundNumber: 8,
					CurrentRootHash:   []byte{0, 1, 3}},
			},
		},
		Author:    "test",
		Signature: []byte{1, 2, 3},
	}
	// Require serialization is equal
	bytes := BytesFromTimeoutVote(timeoutMsg.Timeout, "test", &TimeoutVote{HqcRound: 9, Signature: []byte{1, 2, 3}})
	require.Equal(t, timeoutMsg.Bytes(), bytes)
}

func TestTimeout_IsValid(t *testing.T) {
	type fields struct {
		Round  uint64
		Epoch  uint64
		HighQc *QuorumCert
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Invalid, round is 0",
			fields: fields{
				Round:  0,
				Epoch:  0,
				HighQc: &QuorumCert{},
			},
			wantErr: true,
		},
		{
			name: "Invalid, high QC is nil",
			fields: fields{
				Round:  10,
				Epoch:  0,
				HighQc: nil,
			},
			wantErr: true,
		},
		{
			name: "Invalid, high QC vote info is nil",
			fields: fields{
				Round:  10,
				Epoch:  0,
				HighQc: &QuorumCert{},
			},
			wantErr: true,
		},
		{
			name: "Invalid, high QC round is bigger or equal to timeout round",
			fields: fields{
				Round: 10,
				Epoch: 0,
				HighQc: &QuorumCert{
					VoteInfo:         NewDummyVoteInfo(10),
					LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(10)),
					Signatures:       map[string][]byte{"1": {0, 1, 2, 3}},
				},
			},
			wantErr: true,
		},
		{
			name: "Valid",
			fields: fields{
				Round: 10,
				Epoch: 0,
				HighQc: &QuorumCert{
					VoteInfo:         NewDummyVoteInfo(9),
					LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(9)),
					Signatures:       map[string][]byte{"1": {0, 1, 2, 3}},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &Timeout{
				Round:  tt.fields.Round,
				Epoch:  tt.fields.Epoch,
				HighQc: tt.fields.HighQc,
			}
			if err := x.IsValid(); (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTimeoutMsg_IsValid(t *testing.T) {
	type fields struct {
		Timeout   *Timeout
		Author    string
		Signature []byte
	}
	tests := []struct {
		name    string
		fields  fields
		wantErr bool
	}{
		{
			name: "Invalid, timeout info is nil",
			fields: fields{
				Timeout:   nil,
				Author:    "test",
				Signature: []byte{0, 1, 2},
			},
			wantErr: true,
		},
		{
			name: "Invalid, high QC not valid",
			fields: fields{
				Timeout: &Timeout{
					Round: 10,
					Epoch: 0,
					HighQc: &QuorumCert{
						VoteInfo:         NewDummyVoteInfo(10),
						LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(9)),
						Signatures:       map[string][]byte{"1": {0, 1, 2, 3}},
					},
				},
				Author:    "",
				Signature: []byte{0, 1, 2},
			},
			wantErr: true,
		},
		{
			name: "Invalid, no author",
			fields: fields{
				Timeout: &Timeout{
					Round: 10,
					Epoch: 0,
					HighQc: &QuorumCert{
						VoteInfo:         NewDummyVoteInfo(9),
						LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(9)),
						Signatures:       map[string][]byte{"1": {0, 1, 2, 3}},
					},
				},
				Author:    "",
				Signature: []byte{0, 1, 2},
			},
			wantErr: true,
		},
		{
			name: "Valid",
			fields: fields{
				Timeout: &Timeout{
					Round: 10,
					Epoch: 0,
					HighQc: &QuorumCert{
						VoteInfo:         NewDummyVoteInfo(9),
						LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(9)),
						Signatures:       map[string][]byte{"1": {0, 1, 2, 3}},
					},
				},
				Author:    "test",
				Signature: []byte{0, 1, 2},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &TimeoutMsg{
				Timeout:   tt.fields.Timeout,
				Author:    tt.fields.Author,
				Signature: tt.fields.Signature,
			}
			if err := x.IsValid(); (err != nil) != tt.wantErr {
				t.Errorf("IsValid() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTimeoutMsg_Sign(t *testing.T) {
	s1, _ := testsig.CreateSignerAndVerifier(t)
	// create timeout message without author, verify sign fails
	x := &TimeoutMsg{
		Timeout: &Timeout{
			Round: 10,
			Epoch: 0,
			HighQc: &QuorumCert{
				VoteInfo:         NewDummyVoteInfo(9),
				LedgerCommitInfo: NewDummyCommitInfo(gocrypto.SHA256, NewDummyVoteInfo(9)),
				Signatures:       map[string][]byte{"1": {0, 1, 2, 3}},
			},
		},
		Author: "",
	}
	require.ErrorContains(t, x.Sign(s1), "invalid timeout: missing author")
	require.Nil(t, x.Signature)
	// add author
	x.Author = "test"
	require.NoError(t, x.Sign(s1), "invalid timeout: missing author")
	require.NotNil(t, x.Signature)
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
	highQc := &QuorumCert{
		VoteInfo:         commitQcInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       map[string][]byte{"1": sig1, "2": sig2, "3": sig3},
	}
	// unknown signer
	tmoMsg := NewTimeoutMsg(NewTimeout(votedRound, 0, highQc), "12")
	require.NoError(t, tmoMsg.Sign(s1))
	require.ErrorContains(t, tmoMsg.Verify(quorum, rootTrust), "timeout message verify failed: unable to find public key for author 12")
	// all ok
	tmoMsg = NewTimeoutMsg(NewTimeout(votedRound, 0, highQc), "1")
	require.NoError(t, tmoMsg.Sign(s1))
	require.NoError(t, tmoMsg.Verify(quorum, rootTrust))
	// adjust after signing
	tmoMsg.Timeout.Round = 11
	require.ErrorContains(t, tmoMsg.Verify(quorum, rootTrust), "timeout message verify failed: invalid signature")
}

func TestTimeoutMsg_GetRound(t *testing.T) {
	var tmoMsg *TimeoutMsg = nil
	require.Equal(t, uint64(0), tmoMsg.GetRound())
	tmoMsg = &TimeoutMsg{Timeout: nil}
	require.Equal(t, uint64(0), tmoMsg.GetRound())
	tmoMsg = &TimeoutMsg{Timeout: &Timeout{Round: 10}}
	require.Equal(t, uint64(10), tmoMsg.GetRound())
}
