package abdrc

import (
	gocrypto "crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/crypto/canonicalizer"
	"github.com/alphabill-org/alphabill/internal/testutils"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

type AlwaysValidVerifier struct{}

func (a AlwaysValidVerifier) VerifyBytes(sig []byte, data []byte) error {
	return nil
}

func (a AlwaysValidVerifier) VerifyHash(signature []byte, hash []byte) error {
	return nil
}

func (a AlwaysValidVerifier) VerifyObject(sig []byte, obj canonicalizer.Canonicalizer, opts ...canonicalizer.Option) error {
	return nil
}

func (a AlwaysValidVerifier) MarshalPublicKey() ([]byte, error) {
	return []byte{}, nil
}

func (a AlwaysValidVerifier) UnmarshalPubKey() (gocrypto.PublicKey, error) {
	return []byte{}, nil
}

func TestRecoveryBlock_GetRound(t *testing.T) {
	t.Run("recovery block is nil", func(t *testing.T) {
		var block *RecoveryBlock = nil
		require.EqualValues(t, 0, block.GetRound())
	})
	t.Run("block data is nil", func(t *testing.T) {
		block := &RecoveryBlock{
			Block: nil,
		}
		require.EqualValues(t, 0, block.GetRound())
	})
	t.Run("block data round is 3", func(t *testing.T) {
		block := &RecoveryBlock{
			Block: &drctypes.BlockData{
				Round: 3,
			},
		}
		require.EqualValues(t, 3, block.GetRound())
	})
}

func TestStateMsg_CanRecoverToRound(t *testing.T) {
	t.Run("commit head is nil", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: nil,
		}
		require.ErrorContains(t, sm.CanRecoverToRound(3), "state must have non-nil commit QC")
	})
	t.Run("commit head commit qc is nil", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &RecoveryBlock{CommitQc: nil},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(3), "state must have non-nil commit QC")
	})
	t.Run("commit head block is from later round", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &RecoveryBlock{
				Block:    &drctypes.BlockData{Round: 5},
				CommitQc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}},
			},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(3), "can't recover to round 3 with committed block for round 5")
	})
	t.Run("exact block for round not found", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &RecoveryBlock{
				Block:    &drctypes.BlockData{Round: 5},
				CommitQc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}},
			},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(8), "state has no data block for round 8")
	})
	t.Run("commit head is the block needed", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &RecoveryBlock{
				Block:    &drctypes.BlockData{Round: 5},
				CommitQc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}},
			},
		}
		require.NoError(t, sm.CanRecoverToRound(5))
	})
	t.Run("most common case", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &RecoveryBlock{
				Block:    &drctypes.BlockData{Round: 5},
				CommitQc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}},
			},
			BlockNode: []*RecoveryBlock{
				{
					Block: &drctypes.BlockData{Round: 6},
					Qc:    &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 7}},
				},
				{
					Block: &drctypes.BlockData{Round: 7},
				},
			},
		}
		require.NoError(t, sm.CanRecoverToRound(7))
	})
}

func TestStateMsg_Verify(t *testing.T) {
	t.Run("commit head is nil", func(t *testing.T) {
		sm := &StateMsg{
			Certificates:  nil,
			CommittedHead: nil,
			BlockNode:     nil,
		}
		verifiers := make(map[string]abcrypto.Verifier)
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, 1, verifiers), "commit head is nil")
	})
	t.Run("commit head, commit QC is nil", func(t *testing.T) {
		sm := &StateMsg{
			Certificates: nil,
			CommittedHead: &RecoveryBlock{
				Ir: []*InputData{
					{
						SysID: 1,
						Ir: &types.InputRecord{
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						Sdrh: make([]byte, 32),
					},
				},
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber:       4,
							ParentRoundNumber: 3,
							CurrentRootHash:   test.RandomBytes(32),
							Timestamp:         util.MakeTimestamp(),
						},
						LedgerCommitInfo: &types.UnicitySeal{
							PreviousHash: test.RandomBytes(32),
						},
					},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       7,
						ParentRoundNumber: 6,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         util.MakeTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			BlockNode: nil,
		}
		verifiers := make(map[string]abcrypto.Verifier)
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, 1, verifiers), "invalid commit head, commit qc error: vote info hash verification failed")
	})
	t.Run("commit qc is nil", func(t *testing.T) {
		sm := &StateMsg{
			Certificates: nil,
			CommittedHead: &RecoveryBlock{
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber:       4,
							ParentRoundNumber: 3,
							CurrentRootHash:   test.RandomBytes(32),
							Timestamp:         util.MakeTimestamp(),
						},
						LedgerCommitInfo: &types.UnicitySeal{
							PreviousHash: test.RandomBytes(32),
						},
					},
				},
				Ir: []*InputData{
					{
						SysID: 1,
						Ir: &types.InputRecord{
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						Sdrh: make([]byte, 32),
					},
				},
				CommitQc: nil,
			},
			BlockNode: nil,
		}
		verifiers := make(map[string]abcrypto.Verifier)
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, 1, verifiers), "invalid commit head, commit qc is nil")
	})
	t.Run("invalid block node data", func(t *testing.T) {
		commitVoteInfo := &drctypes.RoundInfo{
			RoundNumber:       7,
			ParentRoundNumber: 6,
			CurrentRootHash:   test.RandomBytes(32),
			Timestamp:         util.MakeTimestamp(),
		}
		sm := &StateMsg{
			Certificates: nil,
			CommittedHead: &RecoveryBlock{
				Ir: []*InputData{
					{
						SysID: 1,
						Ir: &types.InputRecord{
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						Sdrh: make([]byte, 32),
					},
				},
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber:       4,
							ParentRoundNumber: 3,
							CurrentRootHash:   test.RandomBytes(32),
							Timestamp:         util.MakeTimestamp(),
						},
						LedgerCommitInfo: &types.UnicitySeal{
							PreviousHash: test.RandomBytes(32),
							Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
						},
						Signatures: map[string][]byte{"test": test.RandomBytes(65)},
					},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: commitVoteInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: commitVoteInfo.Hash(gocrypto.SHA256),
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			},
			BlockNode: []*RecoveryBlock{{
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{}},
			}},
		}
		verifiers := map[string]abcrypto.Verifier{"test": AlwaysValidVerifier{}}
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, 1, verifiers), "invalid block node: missing input record state")
	})
	t.Run("invalid block node, contains commit QC", func(t *testing.T) {
		commitVoteInfo := &drctypes.RoundInfo{
			RoundNumber:       7,
			ParentRoundNumber: 6,
			CurrentRootHash:   test.RandomBytes(32),
			Timestamp:         util.MakeTimestamp(),
		}
		sm := &StateMsg{
			Certificates: nil,
			CommittedHead: &RecoveryBlock{
				Ir: []*InputData{
					{
						SysID: 1,
						Ir: &types.InputRecord{
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						Sdrh: make([]byte, 32),
					},
				},
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber:       4,
							ParentRoundNumber: 3,
							CurrentRootHash:   test.RandomBytes(32),
							Timestamp:         util.MakeTimestamp(),
						},
						LedgerCommitInfo: &types.UnicitySeal{
							PreviousHash: test.RandomBytes(32),
							Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
						},
						Signatures: map[string][]byte{"test": test.RandomBytes(65)},
					},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: commitVoteInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: commitVoteInfo.Hash(gocrypto.SHA256),
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			},
			BlockNode: []*RecoveryBlock{{
				Ir: []*InputData{
					{
						SysID: 1,
						Ir: &types.InputRecord{
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						Sdrh: make([]byte, 32),
					},
				},
				Block: &drctypes.BlockData{
					Round:   6,
					Payload: &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo: &drctypes.RoundInfo{
							RoundNumber:       5,
							ParentRoundNumber: 4,
							CurrentRootHash:   test.RandomBytes(32),
							Timestamp:         util.MakeTimestamp(),
						},
						LedgerCommitInfo: &types.UnicitySeal{
							PreviousHash: test.RandomBytes(32),
							Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
						},
						Signatures: map[string][]byte{"test": test.RandomBytes(65)},
					},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: commitVoteInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: commitVoteInfo.Hash(gocrypto.SHA256),
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			}},
		}
		verifiers := map[string]abcrypto.Verifier{"test": AlwaysValidVerifier{}}
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, 1, verifiers), "invalid block node, has commit qc set")
	})
}

func TestRecoveryBlock_IsValid(t *testing.T) {
	t.Run("input record state is invalid", func(t *testing.T) {
		r := &RecoveryBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         util.MakeTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: nil,
		}
		require.ErrorContains(t, r.IsValid(), "missing input record state")
	})
	t.Run("input record state is nil", func(t *testing.T) {
		r := &RecoveryBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         util.MakeTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					SysID: 1,
					Ir:    nil,
					Sdrh:  make([]byte, 32),
				},
			},
		}
		require.ErrorContains(t, r.IsValid(), "invalid input record: input record is nil")
	})
	t.Run("block data is nil", func(t *testing.T) {
		r := &RecoveryBlock{
			Block: nil,
			Ir: []*InputData{
				{
					SysID: 1,
					Ir: &types.InputRecord{
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					Sdrh: make([]byte, 32),
				},
			},
		}
		require.ErrorContains(t, r.IsValid(), "block data is nil")
	})
	t.Run("block data is invalid", func(t *testing.T) {
		r := &RecoveryBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:     4,
						CurrentRootHash: test.RandomBytes(32),
						Timestamp:       util.MakeTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					SysID: 1,
					Ir: &types.InputRecord{
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					Sdrh: make([]byte, 32),
				},
			},
		}
		require.ErrorContains(t, r.IsValid(), "block data error: invalid quorum certificate: invalid vote info: parent round number is not assigned")
	})
	t.Run("block data is invalid", func(t *testing.T) {
		r := &RecoveryBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         util.MakeTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					SysID: 1,
					Ir: &types.InputRecord{
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					Sdrh: make([]byte, 32),
				},
			},
		}
		require.NoError(t, r.IsValid())
	})
}

func TestInputData_IsValid(t *testing.T) {
	t.Run("input record is nil", func(t *testing.T) {
		i := &InputData{
			SysID: 0,
			Ir:    nil,
			Sdrh:  nil,
		}
		require.ErrorContains(t, i.IsValid(), "input record is nil")
	})
	t.Run("system description hash is not set", func(t *testing.T) {
		i := &InputData{
			SysID: 0,
			Ir: &types.InputRecord{
				PreviousHash:    test.RandomBytes(32),
				Hash:            test.RandomBytes(32),
				BlockHash:       test.RandomBytes(32),
				SummaryValue:    test.RandomBytes(32),
				RoundNumber:     3,
				SumOfEarnedFees: 10,
			},
			Sdrh: nil,
		}
		require.ErrorContains(t, i.IsValid(), "system descrition hash not set")
	})
	t.Run("ok", func(t *testing.T) {
		i := &InputData{
			SysID: 0,
			Ir: &types.InputRecord{
				PreviousHash:    test.RandomBytes(32),
				Hash:            test.RandomBytes(32),
				BlockHash:       test.RandomBytes(32),
				SummaryValue:    test.RandomBytes(32),
				RoundNumber:     3,
				SumOfEarnedFees: 10,
			},
			Sdrh: test.RandomBytes(32),
		}
		require.NoError(t, i.IsValid())
	})
}
