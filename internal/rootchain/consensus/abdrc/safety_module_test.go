package abdrc

import (
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/abdrc"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
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

func TestSafetyModule_isSafeToVote(t *testing.T) {
	type args struct {
		block       *abtypes.BlockData
		lastRoundTC *abtypes.TimeoutCert
	}
	db := memorydb.New()
	require.NoError(t, db.Write([]byte(highestVotedKey), 3))
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "nil",
			args: args{
				block:       nil,
				lastRoundTC: nil,
			},
			wantErrStr: "block is nil",
		},
		{
			name: "invalid block test, qc is nil",
			args: args{
				block: &abtypes.BlockData{
					Round: 4,
					Qc:    nil},
				lastRoundTC: nil,
			},
			wantErrStr: "block round 4 does not extend from block qc round 0",
		},
		{
			name: "invalid block test, round info is nil",
			args: args{
				block: &abtypes.BlockData{
					Round: 4,
					Qc:    &abtypes.QuorumCert{}},
				lastRoundTC: nil,
			},
			wantErrStr: "block round 4 does not extend from block qc round 0",
		},
		{
			name: "ok",
			args: args{
				block: &abtypes.BlockData{
					Round: 4,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: nil,
			},
		},
		{
			name: "already voted for round 3",
			args: args{
				block: &abtypes.BlockData{
					Round: 3,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: nil,
			},
			wantErrStr: "already voted for round 3, last voted round 3",
		},
		{
			name: "round does not follow qc round",
			args: args{
				block: &abtypes.BlockData{
					Round: 5,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: nil,
			},
			wantErrStr: "block round 5 does not extend from block qc round 3",
		},
		{
			name: "safe to extend from TC, block 5 follows TC round 4 and block QC is equal to TC hqc",
			args: args{
				block: &abtypes.BlockData{
					Round: 5,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{
						Round: 4,
						HighQc: &abtypes.QuorumCert{
							VoteInfo: &abtypes.RoundInfo{
								RoundNumber: 3,
							}}}},
			},
		},
		{
			name: "Not safe to extend from TC, block 5 does not extend TC round 3",
			args: args{
				block: &abtypes.BlockData{
					Round: 5,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{
						Round: 3,
						HighQc: &abtypes.QuorumCert{
							VoteInfo: &abtypes.RoundInfo{
								RoundNumber: 3,
							}}}},
			},
			wantErrStr: "block round 5 does not extend timeout certificate round 3",
		},
		{
			name: "Not safe to extend from TC, block follows TC, but hqc round is higher than block QC round",
			args: args{
				block: &abtypes.BlockData{
					Round: 5,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{
						Round: 4,
						HighQc: &abtypes.QuorumCert{
							VoteInfo: &abtypes.RoundInfo{
								RoundNumber: 4,
							}}},
				},
			},
			wantErrStr: "block qc round 3 is smaller than timeout certificate highest qc round 4",
		},
		{
			name: "safe to extend from TC, block follows TC",
			args: args{
				block: &abtypes.BlockData{
					Round: 4,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 2,
						}}},
				lastRoundTC: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{Round: 3,
						HighQc: &abtypes.QuorumCert{
							VoteInfo: &abtypes.RoundInfo{RoundNumber: 2},
						}}}},
		},
		{
			name: "not safe to extend from TC, invalid TC timeout is nil",
			args: args{
				block: &abtypes.BlockData{
					Round: 4,
					Qc: &abtypes.QuorumCert{
						VoteInfo: &abtypes.RoundInfo{
							RoundNumber: 1,
						}}},
				lastRoundTC: &abtypes.TimeoutCert{
					Timeout: nil}},
			wantErrStr: "block round 4 does not extend timeout certificate round 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &SafetyModule{
				peerID:   "test",
				signer:   nil,
				verifier: nil,
				storage:  db,
			}
			err := s.isSafeToVote(tt.args.block, tt.args.lastRoundTC)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSafetyModule_MakeVote(t *testing.T) {
	s := initSafetyModule(t, "node1")
	dummyRootHash := []byte{1, 2, 3}
	blockData := &abtypes.BlockData{
		Author:    "test",
		Round:     4,
		Epoch:     0,
		Timestamp: 10000,
		Payload:   nil,
		Qc:        nil,
	}
	var tc *abtypes.TimeoutCert = nil
	vote, err := s.MakeVote(blockData, dummyRootHash, nil, tc)
	require.ErrorContains(t, err, "block is missing quorum certificate")
	require.Nil(t, vote)
	// try to make a successful dummy vote
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	// create a dummy QC
	blockData.Qc = abtypes.NewQuorumCertificate(voteInfo, nil)
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
	require.ErrorContains(t, err, "not safe to vote")
	require.Nil(t, vote)

}

func TestSafetyModule_SignProposal(t *testing.T) {
	s := initSafetyModule(t, "node1")
	// create a dummy proposal message
	proposal := &abdrc.ProposalMsg{
		Block: &abtypes.BlockData{
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
	require.ErrorContains(t, s.Sign(proposal), "missing payload")
	// add empty payload
	proposal.Block.Payload = &abtypes.Payload{Requests: nil}
	// still missing QC
	require.ErrorContains(t, s.Sign(proposal), "missing quorum certificate")
	// create dummy QC
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	qc := abtypes.NewQuorumCertificate(voteInfo, nil)
	// add some dummy signatures
	qc.Signatures = map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	proposal.Block.Qc = qc
	require.NoError(t, s.Sign(proposal))
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
	// previous round did not time out
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	qc := abtypes.NewQuorumCertificate(voteInfo, nil)
	qc.Signatures = map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	tmoMsg := &abdrc.TimeoutMsg{
		Timeout: &abtypes.Timeout{Epoch: 0,
			Round:  3,
			HighQc: qc,
		},
		Author: "test",
	}
	require.ErrorContains(t, s.SignTimeout(tmoMsg, nil), "timeout message not valid, invalid timeout data: timeout round (3) must be greater than high QC round (3)")
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
		block        *abtypes.BlockData
		voteInfoHash []byte
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *types.UnicitySeal
	}{
		{
			name:   "To be committed",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &abtypes.BlockData{
				Round: 3,
				Qc: &abtypes.QuorumCert{
					VoteInfo: &abtypes.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1, CurrentRootHash: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: &types.UnicitySeal{PreviousHash: []byte{2, 2, 2, 2}, RootChainRoundNumber: 2, Hash: []byte{0, 1, 2, 3}},
		},
		{
			name:   "Not to be committed",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &abtypes.BlockData{
				Round: 3,
				Qc: &abtypes.QuorumCert{
					VoteInfo: &abtypes.RoundInfo{RoundNumber: 1, ParentRoundNumber: 0, CurrentRootHash: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: &types.UnicitySeal{PreviousHash: []byte{2, 2, 2, 2}},
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
		block *abtypes.BlockData
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
			args: args{block: &abtypes.BlockData{
				Round: 3,
				Qc: &abtypes.QuorumCert{
					VoteInfo: &abtypes.RoundInfo{RoundNumber: 2, CurrentRootHash: []byte{0, 1, 2, 3}},
				},
			}},
			want: []byte{0, 1, 2, 3},
		},
		{
			name:   "Not candidate, block round does not follow QC round",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &abtypes.BlockData{
				Round: 3,
				Qc: &abtypes.QuorumCert{
					VoteInfo: &abtypes.RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 2, 3}},
				},
			}},
			want: nil,
		},
		{
			name:   "Not candidate, QC is nil",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &abtypes.BlockData{
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
			if tt.want == nil {
				require.Nil(t, s.isCommitCandidate(tt.args.block))
			} else {
				require.NotNil(t, s.isCommitCandidate(tt.args.block))
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
		round   uint64               // timeout round
		qcRound uint64               // timeout highest seen QC round
		tc      *abtypes.TimeoutCert // previous round TC
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantErrStr string
	}{
		{
			name:   "OK",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{round: 2, qcRound: 1,
				tc: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{Round: 2,
						HighQc: &abtypes.QuorumCert{
							VoteInfo: &abtypes.RoundInfo{RoundNumber: 1},
						}}}},
			wantErrStr: "",
		},
		{
			name:       "Not safe to timeout, last round was not TC, but QC round is smaller than the QC we have seen",
			fields:     fields{highestVotedRound: 2, highestQcRound: 2, signer: nil},
			args:       args{round: 2, qcRound: 1, tc: nil},
			wantErrStr: "qc round 1 is smaller than highest qc round 2 seen",
		},
		{
			name:       "Already voted for round 2 and can vote again for timeout",
			fields:     fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args:       args{round: 2, qcRound: 1, tc: nil},
			wantErrStr: "",
		},
		{
			name:   "try timeout a past round",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{round: 2, qcRound: 2,
				tc: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{Round: 3,
						HighQc: &abtypes.QuorumCert{
							VoteInfo: &abtypes.RoundInfo{RoundNumber: 1},
						}}}},
			wantErrStr: "timeout round 2 is in the past, highest voted round 1, hqc round 2",
		},
		{
			name:       "round does not follow QC",
			fields:     fields{highestVotedRound: 2, highestQcRound: 2, signer: nil},
			args:       args{round: 4, qcRound: 2, tc: nil},
			wantErrStr: "round 4 does not follow last qc round 2 or tc round 0",
		},
		{
			name:   "round does not follow TC",
			fields: fields{highestVotedRound: 2, highestQcRound: 2, signer: nil},
			args: args{round: 5, qcRound: 2,
				tc: &abtypes.TimeoutCert{
					Timeout: &abtypes.Timeout{Round: 3,
						HighQc: &abtypes.QuorumCert{
							VoteInfo: &abtypes.RoundInfo{RoundNumber: 2},
						}}}},
			wantErrStr: "round 5 does not follow last qc round 2 or tc round 3",
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
			err := s.isSafeToTimeout(tt.args.round, tt.args.qcRound, tt.args.tc)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
