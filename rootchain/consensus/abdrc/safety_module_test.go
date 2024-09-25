package abdrc

import (
	"reflect"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/stretchr/testify/require"
)

func initSafetyModule(t *testing.T, id string) *SafetyModule {
	t.Helper()
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	db, err := memorydb.New()
	require.NoError(t, err)
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
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	db, err := memorydb.New()
	require.NoError(t, err)
	s, err := NewSafetyModule("1", signer, db)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, uint64(defaultHighestQcRound), s.GetHighestQcRound())
	require.Equal(t, uint64(defaultHighestVotedRound), s.GetHighestVotedRound())
}

func TestNewSafetyModule_WithStorageNotEmpty(t *testing.T) {
	// creates and initiates the bolt store backend, and saves initial state
	db, err := memorydb.New()
	require.NoError(t, err)
	hQcRound := uint64(3)
	hVotedRound := uint64(4)
	require.NoError(t, db.Write([]byte(highestVotedKey), hVotedRound))
	require.NoError(t, db.Write([]byte(highestQcKey), hQcRound))
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	s, err := NewSafetyModule("1", signer, db)
	require.NoError(t, err)
	require.NotNil(t, s)
	require.Equal(t, uint64(3), s.GetHighestQcRound())
	require.Equal(t, uint64(4), s.GetHighestVotedRound())
}

func TestSafetyModule_isSafeToVote(t *testing.T) {
	type args struct {
		block       *drctypes.BlockData
		lastRoundTC *drctypes.TimeoutCert
	}
	db, err := memorydb.New()
	require.NoError(t, err)
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
				block: &drctypes.BlockData{
					Round: 4,
					Qc:    nil},
				lastRoundTC: nil,
			},
			wantErrStr: "block round 4 does not extend from block qc round 0",
		},
		{
			name: "invalid block test, round info is nil",
			args: args{
				block: &drctypes.BlockData{
					Round: 4,
					Qc:    &drctypes.QuorumCert{}},
				lastRoundTC: nil,
			},
			wantErrStr: "block round 4 does not extend from block qc round 0",
		},
		{
			name: "ok",
			args: args{
				block: &drctypes.BlockData{
					Round: 4,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: nil,
			},
		},
		{
			name: "already voted for round 3",
			args: args{
				block: &drctypes.BlockData{
					Round: 3,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: nil,
			},
			wantErrStr: "already voted for round 3, last voted round 3",
		},
		{
			name: "round does not follow qc round",
			args: args{
				block: &drctypes.BlockData{
					Round: 5,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: nil,
			},
			wantErrStr: "block round 5 does not extend from block qc round 3",
		},
		{
			name: "safe to extend from TC, block 5 follows TC round 4 and block QC is equal to TC hqc",
			args: args{
				block: &drctypes.BlockData{
					Round: 5,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: &drctypes.TimeoutCert{
					Timeout: &drctypes.Timeout{
						Round: 4,
						HighQc: &drctypes.QuorumCert{
							VoteInfo: &drctypes.RoundInfo{
								RoundNumber: 3,
							}}}},
			},
		},
		{
			name: "Not safe to extend from TC, block 5 does not extend TC round 3",
			args: args{
				block: &drctypes.BlockData{
					Round: 5,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: &drctypes.TimeoutCert{
					Timeout: &drctypes.Timeout{
						Round: 3,
						HighQc: &drctypes.QuorumCert{
							VoteInfo: &drctypes.RoundInfo{
								RoundNumber: 3,
							}}}},
			},
			wantErrStr: "block round 5 does not extend timeout certificate round 3",
		},
		{
			name: "Not safe to extend from TC, block follows TC, but hqc round is higher than block QC round",
			args: args{
				block: &drctypes.BlockData{
					Round: 5,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 3,
						}}},
				lastRoundTC: &drctypes.TimeoutCert{
					Timeout: &drctypes.Timeout{
						Round: 4,
						HighQc: &drctypes.QuorumCert{
							VoteInfo: &drctypes.RoundInfo{
								RoundNumber: 4,
							}}},
				},
			},
			wantErrStr: "block qc round 3 is smaller than timeout certificate highest qc round 4",
		},
		{
			name: "safe to extend from TC, block follows TC",
			args: args{
				block: &drctypes.BlockData{
					Round: 4,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 2,
						}}},
				lastRoundTC: &drctypes.TimeoutCert{
					Timeout: &drctypes.Timeout{Round: 3,
						HighQc: &drctypes.QuorumCert{
							VoteInfo: &drctypes.RoundInfo{RoundNumber: 2},
						}}}},
		},
		{
			name: "not safe to extend from TC, invalid TC timeout is nil",
			args: args{
				block: &drctypes.BlockData{
					Round: 4,
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber: 1,
						}}},
				lastRoundTC: &drctypes.TimeoutCert{
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
	blockData := &drctypes.BlockData{
		Author:    "test",
		Round:     4,
		Epoch:     0,
		Timestamp: 10000,
		Payload:   nil,
		Qc:        nil,
	}
	var tc *drctypes.TimeoutCert = nil
	vote, err := s.MakeVote(blockData, dummyRootHash, nil, tc)
	require.ErrorContains(t, err, "block is missing quorum certificate")
	require.Nil(t, vote)
	// try to make a successful dummy vote
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	// create a dummy QC
	blockData.Qc = drctypes.NewQuorumCertificate(voteInfo, nil)
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
		Block: &drctypes.BlockData{
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
	proposal.Block.Payload = &drctypes.Payload{Requests: nil}
	// still missing QC
	require.ErrorContains(t, s.Sign(proposal), "missing quorum certificate")
	// create dummy QC
	voteInfo := NewDummyVoteInfo(3, []byte{0, 1, 2, 3})
	qc := drctypes.NewQuorumCertificate(voteInfo, nil)
	// add some dummy signatures
	qc.Signatures = map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	proposal.Block.Qc = qc
	require.NoError(t, s.Sign(proposal))
	require.Greater(t, len(proposal.Signature), 1)
}

func TestSafetyModule_SignTimeout(t *testing.T) {
	signer, err := abcrypto.NewInMemorySecp256K1Signer()
	require.Nil(t, err)
	db, err := memorydb.New()
	require.NoError(t, err)
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
	qc := drctypes.NewQuorumCertificate(voteInfo, nil)
	qc.Signatures = map[string][]byte{"1": {1, 2}, "2": {1, 2}, "3": {1, 2}}
	tmoMsg := &abdrc.TimeoutMsg{
		Timeout: &drctypes.Timeout{Epoch: 0,
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
		signer            abcrypto.Signer
	}
	type args struct {
		block        *drctypes.BlockData
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
			args: args{block: &drctypes.BlockData{
				Round: 3,
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1, CurrentRootHash: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
				seal.PreviousHash = []byte{2, 2, 2, 2}
				seal.RootChainRoundNumber = 2
				seal.Hash = []byte{0, 1, 2, 3}
			}),
		},
		{
			name:   "Not to be committed",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &drctypes.BlockData{
				Round: 3,
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{RoundNumber: 1, ParentRoundNumber: 0, CurrentRootHash: []byte{0, 1, 2, 3}},
				}},
				voteInfoHash: []byte{2, 2, 2, 2}},
			want: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
				seal.PreviousHash = []byte{2, 2, 2, 2}
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := memorydb.New()
			require.NoError(t, err)
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
		signer            abcrypto.Signer
	}
	type args struct {
		block *drctypes.BlockData
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
			args: args{block: &drctypes.BlockData{
				Round: 3,
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{RoundNumber: 2, CurrentRootHash: []byte{0, 1, 2, 3}},
				},
			}},
			want: []byte{0, 1, 2, 3},
		},
		{
			name:   "Not candidate, block round does not follow QC round",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &drctypes.BlockData{
				Round: 3,
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{RoundNumber: 1, CurrentRootHash: []byte{0, 1, 2, 3}},
				},
			}},
			want: nil,
		},
		{
			name:   "Not candidate, QC is nil",
			fields: fields{highestVotedRound: 2, highestQcRound: 1, signer: nil},
			args: args{block: &drctypes.BlockData{
				Round: 3,
				Qc:    nil,
			}},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := memorydb.New()
			require.NoError(t, err)
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
	t.Run("OK", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		var highestVotedRound uint64 = 2
		var highestQcRound uint64 = 1
		require.NoError(t, db.Write([]byte(highestVotedKey), highestVotedRound))
		require.NoError(t, db.Write([]byte(highestQcKey), highestQcRound))
		s := &SafetyModule{
			storage: db,
		}
		tc := &drctypes.TimeoutCert{
			Timeout: &drctypes.Timeout{Round: 2,
				HighQc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{RoundNumber: 1},
				}}}
		require.NoError(t, s.isSafeToTimeout(2, 1, tc))
	})
	t.Run("not safe - last round was not TC, but QC round is smaller than the QC we have seen", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		var highestVotedRound uint64 = 2
		var highestQcRound uint64 = 2
		require.NoError(t, db.Write([]byte(highestVotedKey), highestVotedRound))
		require.NoError(t, db.Write([]byte(highestQcKey), highestQcRound))
		s := &SafetyModule{
			storage: db,
		}
		require.ErrorContains(t, s.isSafeToTimeout(2, 1, nil), "qc round 1 is smaller than highest qc round 2 seen")
	})
	t.Run("ok - already voted for round 2 and can vote again for timeout", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		var highestVotedRound uint64 = 2
		var highestQcRound uint64 = 1
		require.NoError(t, db.Write([]byte(highestVotedKey), highestVotedRound))
		require.NoError(t, db.Write([]byte(highestQcKey), highestQcRound))
		s := &SafetyModule{
			storage: db,
		}
		require.NoError(t, s.isSafeToTimeout(2, 1, nil))
	})
	t.Run("not safe - timeout round is in past", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		var highestVotedRound uint64 = 2
		var highestQcRound uint64 = 1
		require.NoError(t, db.Write([]byte(highestVotedKey), highestVotedRound))
		require.NoError(t, db.Write([]byte(highestQcKey), highestQcRound))
		s := &SafetyModule{
			storage: db,
		}
		require.ErrorContains(t, s.isSafeToTimeout(2, 2, nil), "timeout round 2 is in the past, timeout msg high qc is for round 2")
	})
	t.Run("not safe - already signed vote for round", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		var highestVotedRound uint64 = 3
		var highestQcRound uint64 = 1
		require.NoError(t, db.Write([]byte(highestVotedKey), highestVotedRound))
		require.NoError(t, db.Write([]byte(highestQcKey), highestQcRound))
		s := &SafetyModule{
			storage: db,
		}
		require.ErrorContains(t, s.isSafeToTimeout(2, 1, nil), "timeout round 2 is in the past, already signed vote for round 3")
	})
	t.Run("not safe - round does not follow QC", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		var highestVotedRound uint64 = 2
		var highestQcRound uint64 = 2
		require.NoError(t, db.Write([]byte(highestVotedKey), highestVotedRound))
		require.NoError(t, db.Write([]byte(highestQcKey), highestQcRound))
		s := &SafetyModule{
			storage: db,
		}
		require.ErrorContains(t, s.isSafeToTimeout(4, 2, nil), "round 4 does not follow last qc round 2 or tc round 0")
	})
	t.Run("not safe - round does not follow TC", func(t *testing.T) {
		db, err := memorydb.New()
		require.NoError(t, err)
		var highestVotedRound uint64 = 2
		var highestQcRound uint64 = 2
		require.NoError(t, db.Write([]byte(highestVotedKey), highestVotedRound))
		require.NoError(t, db.Write([]byte(highestQcKey), highestQcRound))
		s := &SafetyModule{
			storage: db,
		}
		lastRoundTC := &drctypes.TimeoutCert{
			Timeout: &drctypes.Timeout{Round: 3,
				HighQc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{RoundNumber: 2},
				}}}
		require.ErrorContains(t, s.isSafeToTimeout(5, 2, lastRoundTC), "round 5 does not follow last qc round 2 or tc round 3")
	})
}
