package distributed

import (
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/stretchr/testify/require"
)

func initSafetyModule(t *testing.T, id string) *SafetyModule {
	t.Helper()
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	db := memorydb.New()
	safety, err := NewSafetyModule(id, signer, db)
	require.NoError(t, err)
	require.NotNil(t, safety)
	require.NotNil(t, safety.verifier)
	return safety
}

func TestIsConsecutive(t *testing.T) {
	const currentRound = 4
	// block is deemed consecutive if it follows current round 4 i.e. block with round 5 is consecutive
	require.False(t, isConsecutive(4, currentRound))
	require.True(t, isConsecutive(5, currentRound))
	require.False(t, isConsecutive(6, currentRound))
}

func TestNewSafetyModule(t *testing.T) {
	safety := initSafetyModule(t, "node1")
	require.Equal(t, uint64(defaultHighestQcRound), safety.GetHighestQcRound())
	require.Equal(t, uint64(defaultHighestVotedRound), safety.GetHighestVotedRound())
}

func TestNewSafetyModule_WithStorageEmpty(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	db := memorydb.New()
	s, err := NewSafetyModule("1", signer, db)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, uint64(defaultHighestQcRound), s.GetHighestQcRound())
	require.Equal(t, uint64(defaultHighestVotedRound), s.GetHighestVotedRound())
}

func TestNewSafetyModule_WithStorageNotEmpty(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	db := memorydb.New()
	hQcRound := uint64(3)
	hVotedRound := uint64(4)
	require.NoError(t, db.Write([]byte(highestVotedKey), hVotedRound))
	require.NoError(t, db.Write([]byte(highestQcKey), hQcRound))
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	s, err := NewSafetyModule("1", signer, db)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, uint64(3), s.GetHighestQcRound())
	require.Equal(t, uint64(4), s.GetHighestVotedRound())
}

func TestSafetyModule_IsSafeToVote(t *testing.T) {
	var blockRound uint64 = 4
	s := initSafetyModule(t, "node1")
	// Assume last round was not TC - TC is nil
	// then it is safe to vote:
	// 1. must vote in monotonically increasing rounds
	// 2. must extend a smaller round
	var tc *atomic_broadcast.TimeoutCert = nil
	require.False(t, s.isSafeToVote(blockRound, 2, tc))
	require.True(t, s.isSafeToVote(blockRound, 3, tc))
	require.False(t, s.isSafeToVote(blockRound, 4, tc))
	// create dummy TC for last round
	voteInfo := NewDummyVoteInfo(4, []byte{0, 1, 2, 3})
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	tc = NewDummyTc(4, qc)
	// Is safe because block follows directly to QC
	require.True(t, s.isSafeToVote(blockRound, 3, tc))
	// Not safe because block round does not follow TC round
	require.False(t, s.isSafeToVote(blockRound, 2, tc))
	tc = NewDummyTc(3, qc)
	// Not safe because TC highest QC round seen is bigger than QC round
	require.False(t, s.isSafeToVote(blockRound, 2, tc))
	// Safe, block follows TC and the QC round is >= TC higest QC round seen by the group
	voteInfo = NewDummyVoteInfo(1, []byte{0, 1, 2, 3})
	qc = atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	tc = NewDummyTc(3, qc)
	require.True(t, s.isSafeToVote(blockRound, 2, tc))
}

func TestSafetyModule_MakeVote(t *testing.T) {
	s := initSafetyModule(t, "node1")
	dummyRootHash := []byte{1, 2, 3}
	blockData := &atomic_broadcast.BlockData{
		Author:    "test",
		Round:     4,
		Epoch:     0,
		Timestamp: 10000,
		Payload:   nil,
		Qc:        nil,
	}
	var tc *atomic_broadcast.TimeoutCert = nil
	vote, err := s.MakeVote(blockData, dummyRootHash, nil, tc)
	require.Error(t, atomic_broadcast.ErrMissingQuorumCertificate, err)
	require.Nil(t, vote)
	// try to make a successful dummy vote
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	// create a dummy QC
	blockData.Qc = atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	vote, err = s.MakeVote(blockData, dummyRootHash, nil, tc)
	require.NoError(t, err)
	require.NotNil(t, vote)
	require.Equal(t, "node1", vote.Author)
	require.Greater(t, len(vote.Signature), 1)
	require.NotNil(t, vote.LedgerCommitInfo)
	require.Equal(t, uint64(3), s.GetHighestQcRound())
	require.Equal(t, uint64(4), s.GetHighestVotedRound())
	// try to sign the same vote again
	vote, err = s.MakeVote(blockData, dummyRootHash, nil, tc)
	// only allowed to vote for monotonically increasing rounds
	require.ErrorContains(t, err, "Not safe to vote")
	require.Nil(t, vote)

}

func TestSafetyModule_SignProposal(t *testing.T) {
	s := initSafetyModule(t, "node1")
	// create a dummy proposal message
	proposal := &atomic_broadcast.ProposalMsg{
		Block: &atomic_broadcast.BlockData{
			Author:    "test",
			Round:     4,
			Epoch:     0,
			Timestamp: 10000,
			Payload:   nil,
			Qc:        nil,
		},
		LastRoundTc: nil,
	}
	// invalid block missing payload and QC
	require.ErrorIs(t, s.SignProposal(proposal), atomic_broadcast.ErrMissingPayload)
	// add empty payload
	proposal.Block.Payload = &atomic_broadcast.Payload{Requests: nil}
	// still missing QC
	require.ErrorIs(t, s.SignProposal(proposal), atomic_broadcast.ErrMissingQuorumCertificate)
	// create dummy QC
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	// add some dummy signatures
	qc.Signatures = map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	proposal.Block.Qc = qc
	require.NoError(t, s.SignProposal(proposal))
	require.Greater(t, len(proposal.Signature), 1)
}

func TestSafetyModule_SignTimeout(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	db := memorydb.New()
	hQcRound := uint64(2)
	hVotedRound := uint64(3)
	require.NoError(t, db.Write([]byte(highestVotedKey), hVotedRound))
	require.NoError(t, db.Write([]byte(highestQcKey), hQcRound))
	s := &SafetyModule{
		signer:  signer,
		storage: db,
	}
	require.NotNil(t, s)
	// previous round did not timeout
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	qc.Signatures = map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	tmoMsg := &atomic_broadcast.TimeoutMsg{
		Timeout: &atomic_broadcast.Timeout{Epoch: 0,
			Round:  3,
			HighQc: qc,
		},
		Author: "test",
	}
	require.ErrorContains(t, s.SignTimeout(tmoMsg, nil), "not safe to timeout")
	require.Nil(t, tmoMsg.Signature)
	tmoMsg.Timeout.Round = 4
	require.NoError(t, s.SignTimeout(tmoMsg, nil))
	require.NotNil(t, tmoMsg.Signature)
}

func TestSafetyModule_constructLedgerCommitInfo(t *testing.T) {
	type fields struct {
		highestVotedRound uint64
		highestQcRound    uint64
		signer            crypto.Signer
	}
	type args struct {
		block        *atomic_broadcast.BlockData
		voteInfoHash []byte
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *certificates.CommitInfo
	}{
		{
			name:   "To be committed",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &certificates.RootRoundInfo{RoundNumber: 2, ParentRoundNumber: 1, CurrentRootHash: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: &certificates.CommitInfo{RootRoundInfoHash: []byte{2, 2, 2, 2}, RootHash: []byte{0, 1, 2, 3}},
		},
		{
			name:   "Not to be committed",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &certificates.RootRoundInfo{RoundNumber: 1, ParentRoundNumber: 0, CurrentRootHash: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: &certificates.CommitInfo{RootRoundInfoHash: []byte{2, 2, 2, 2}, RootHash: nil},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := memorydb.New()
			require.NoError(t, db.Write([]byte(highestVotedKey), tt.fields.highestVotedRound))
			require.NoError(t, db.Write([]byte(highestQcKey), tt.fields.highestQcRound))
			s := &SafetyModule{
				signer:  tt.fields.signer,
				storage: db,
			}
			if got := s.constructCommitInfo(tt.args.block, tt.args.voteInfoHash); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("constructCommitInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafetyModule_isCommitCandidate(t *testing.T) {
	type fields struct {
		highestVotedRound uint64
		highestQcRound    uint64
		signer            crypto.Signer
	}
	type args struct {
		block *atomic_broadcast.BlockData
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []byte
	}{
		{
			name:   "Is candidate",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &certificates.RootRoundInfo{RoundNumber: 2, CurrentRootHash: []byte{0, 1, 2, 3}},
				},
			}},
			want: []byte{0, 1, 2, 3},
		},
		{
			name:   "Not candidate, block round does not follow QC round",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc: &atomic_broadcast.QuorumCert{
					VoteInfo: &certificates.RootRoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 2, 3}},
				},
			}},
			want: nil,
		},
		{
			name:   "Not candidate, QC is nil",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &atomic_broadcast.BlockData{
				Round: 3,
				Qc:    nil,
			}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := memorydb.New()
			require.NoError(t, db.Write([]byte(highestVotedKey), tt.fields.highestVotedRound))
			require.NoError(t, db.Write([]byte(highestQcKey), tt.fields.highestQcRound))

			s := &SafetyModule{
				signer:  tt.fields.signer,
				storage: db,
			}
			if got := s.isCommitCandidate(tt.args.block); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("isCommitCandidate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSafetyModule_isSafeToTimeout(t *testing.T) {
	type fields struct {
		highestVotedRound uint64
		highestQcRound    uint64
		signer            crypto.Signer
	}
	type args struct {
		round   uint64                        // timeout round
		qcRound uint64                        // timeout highest seen QC round
		tc      *atomic_broadcast.TimeoutCert // previous round TC
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name:   "OK",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{round: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						HighQc: &atomic_broadcast.QuorumCert{
							VoteInfo: &certificates.RootRoundInfo{RoundNumber: 1},
						}}}},
			want: true,
		},
		{
			name:   "Not safe to timeout, last round was not TC, but QC round is smaller than the QC we have seen",
			fields: fields{highestVotedRound: 2, highestQcRound: 2, signer: nil},
			args:   args{round: 2, qcRound: 1, tc: nil},
			want:   false,
		},
		{
			name:   "Already voted for round 2 and can vote again for timeout",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args:   args{round: 2, qcRound: 1, tc: nil},
			want:   true,
		},
		{
			name:   "round does not follow QC or TC",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{round: 2, qcRound: 2,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 3,
						HighQc: &atomic_broadcast.QuorumCert{
							VoteInfo: &certificates.RootRoundInfo{RoundNumber: 1},
						}}}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := memorydb.New()
			require.NoError(t, db.Write([]byte(highestVotedKey), tt.fields.highestVotedRound))
			require.NoError(t, db.Write([]byte(highestQcKey), tt.fields.highestQcRound))

			s := &SafetyModule{
				signer:  tt.fields.signer,
				storage: db,
			}
			if got := s.isSafeToTimeout(tt.args.round, tt.args.qcRound, tt.args.tc); got != tt.want {
				t.Errorf("isSafeToTimeout() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isSaveToExtend(t *testing.T) {
	type args struct {
		blockRound uint64
		qcRound    uint64
		tc         *atomic_broadcast.TimeoutCert
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "Safe to extend from TC, block follows TC",
			args: args{blockRound: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 1,
						HighQc: &atomic_broadcast.QuorumCert{
							VoteInfo: &certificates.RootRoundInfo{RoundNumber: 1},
						}}}},
			want: true,
		},
		{
			name: "Not safe to extend from TC, block does not follow TC",
			args: args{blockRound: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						HighQc: &atomic_broadcast.QuorumCert{
							VoteInfo: &certificates.RootRoundInfo{RoundNumber: 1},
						}}}},
			want: false,
		},
		{
			name: "Not safe to extend from TC, block does not follow TC",
			args: args{blockRound: 2, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 3,
						HighQc: &atomic_broadcast.QuorumCert{
							VoteInfo: &certificates.RootRoundInfo{RoundNumber: 1},
						}}}},
			want: false,
		},
		{
			name: "Not safe to extend from TC, block follows TC, but hqc round is higher than qc round",
			args: args{blockRound: 3, qcRound: 1,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						HighQc: &atomic_broadcast.QuorumCert{
							VoteInfo: &certificates.RootRoundInfo{RoundNumber: 3},
						}}}},
			want: false,
		},
		{
			name: "Safe to extend from TC, block follows TC and hqc round is equal to qc round",
			args: args{blockRound: 3, qcRound: 3,
				tc: &atomic_broadcast.TimeoutCert{
					Timeout: &atomic_broadcast.Timeout{Round: 2,
						HighQc: &atomic_broadcast.QuorumCert{
							VoteInfo: &certificates.RootRoundInfo{RoundNumber: 3},
						}}}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeToExtend(tt.args.blockRound, tt.args.qcRound, tt.args.tc); got != tt.want {
				t.Errorf("isSafeToExtend() = %v, want %v", got, tt.want)
			}
		})
	}
}
