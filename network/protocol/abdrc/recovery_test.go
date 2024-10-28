package abdrc

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/crypto/canonicalizer"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
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
		var block *CommittedBlock = nil
		require.EqualValues(t, 0, block.GetRound())
	})
	t.Run("block data is nil", func(t *testing.T) {
		block := &CommittedBlock{
			Block: nil,
		}
		require.EqualValues(t, 0, block.GetRound())
	})
	t.Run("block data round is 3", func(t *testing.T) {
		block := &CommittedBlock{
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
		require.ErrorContains(t, sm.CanRecoverToRound(3), "committed block is nil")
	})
	t.Run("commit head commit qc is nil", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(3), "state has no data block for round 3")
	})
	t.Run("commit head block is from later round", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{
				Block: &drctypes.BlockData{
					Round: 5,
					Qc:    &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 4}},
				},
			},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(3), "can't recover to round 3 with committed block for round 5")
	})
	t.Run("exact block for round not found", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{
				Block: &drctypes.BlockData{
					Round: 5,
					Qc:    &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 4}},
				},
			},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(8), "state has no data block for round 8")
	})
	t.Run("commit head is the block needed", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{
				Block: &drctypes.BlockData{
					Round: 5,
					Qc:    &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 4}},
				},
			},
		}
		require.NoError(t, sm.CanRecoverToRound(5))
	})
	t.Run("most common case", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{
				Block: &drctypes.BlockData{
					Round: 5,
					Qc:    &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 4}},
				},
			},
			BlockData: []*drctypes.BlockData{
				{
					Round: 6,
					Qc:    &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 5}},
				},
				{
					Round: 7,
					Qc:    &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 6}},
				},
			},
		}
		require.NoError(t, sm.CanRecoverToRound(7))
	})
}

func TestStateMsg_Verify(t *testing.T) {
	r4vInfo := &drctypes.RoundInfo{
		RoundNumber:       4,
		ParentRoundNumber: 3,
		CurrentRootHash:   test.RandomBytes(32),
		Timestamp:         types.NewTimestamp(),
	}
	r5vInfo := &drctypes.RoundInfo{
		RoundNumber:       5,
		ParentRoundNumber: 4,
		CurrentRootHash:   test.RandomBytes(32),
		Timestamp:         types.NewTimestamp(),
	}
	r6vInfo := &drctypes.RoundInfo{
		RoundNumber:       6,
		ParentRoundNumber: 5,
		CurrentRootHash:   test.RandomBytes(32),
		Timestamp:         types.NewTimestamp(),
	}
	t.Run("commit head is nil", func(t *testing.T) {
		sm := &StateMsg{
			ShardInfo:     nil,
			CommittedHead: nil,
			BlockData:     nil,
		}
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, &types.RootTrustBaseV1{Version: 1}), "commit head is nil")
	})
	t.Run("commit head, invalid block", func(t *testing.T) {
		sm := &StateMsg{
			ShardInfo: nil,
			CommittedHead: &CommittedBlock{
				Ir: []*InputData{
					{
						Partition: 1,
						Ir: &types.InputRecord{Version: 1,
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						PDRHash: make([]byte, 32),
					},
				},
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{},
					Qc:      nil,
				},
				Qc: &drctypes.QuorumCert{
					VoteInfo: r5vInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						Version:      1,
						PreviousHash: r5vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: r6vInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						Version:      1,
						PreviousHash: r6vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			},
			BlockData: nil,
		}
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, &types.RootTrustBaseV1{Version: 1}), "invalid commit head: block data error: proposed block is missing quorum certificate")
	})
	t.Run("commit head, invalid QC", func(t *testing.T) {
		sm := &StateMsg{
			ShardInfo: nil,
			CommittedHead: &CommittedBlock{
				Ir: []*InputData{
					{
						Partition: 1,
						Ir: &types.InputRecord{Version: 1,
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						PDRHash: make([]byte, 32),
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
							Timestamp:         types.NewTimestamp(),
						},
						LedgerCommitInfo: &types.UnicitySeal{Version: 1,
							PreviousHash: test.RandomBytes(32),
						},
					},
				},
				Qc: &drctypes.QuorumCert{
					VoteInfo: r5vInfo,
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: r5vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: r6vInfo,
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: r6vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			},
			BlockData: nil,
		}
		require.EqualError(t, sm.Verify(gocrypto.SHA256, &types.RootTrustBaseV1{Version: 1}), "block qc verification error: vote info hash verification failed")
	})
	t.Run("invalid block node data", func(t *testing.T) {
		sm := &StateMsg{
			ShardInfo: nil,
			CommittedHead: &CommittedBlock{
				Ir: []*InputData{
					{
						Partition: 1,
						Ir: &types.InputRecord{Version: 1,
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						PDRHash: make([]byte, 32),
					},
				},
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo: r4vInfo,
						LedgerCommitInfo: &types.UnicitySeal{Version: 1,
							PreviousHash: r4vInfo.Hash(gocrypto.SHA256),
							Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
						},
						Signatures: map[string][]byte{"test": test.RandomBytes(65)},
					},
				},
				Qc: &drctypes.QuorumCert{
					VoteInfo: r5vInfo,
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: r5vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: r6vInfo,
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: r6vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			},
			BlockData: []*drctypes.BlockData{{
				Round:   5,
				Payload: &drctypes.Payload{}},
			},
		}
		tb := testtb.NewTrustBase(t, AlwaysValidVerifier{})
		require.ErrorContains(t, sm.Verify(gocrypto.SHA256, tb), "invalid block node: proposed block is missing quorum certificate")
	})
	t.Run("ok", func(t *testing.T) {
		sm := &StateMsg{
			ShardInfo: nil,
			CommittedHead: &CommittedBlock{
				Ir: []*InputData{
					{
						Partition: 1,
						Ir: &types.InputRecord{Version: 1,
							PreviousHash:    test.RandomBytes(32),
							Hash:            test.RandomBytes(32),
							BlockHash:       test.RandomBytes(32),
							SummaryValue:    test.RandomBytes(32),
							RoundNumber:     3,
							SumOfEarnedFees: 10,
						},
						PDRHash: make([]byte, 32),
					},
				},
				Block: &drctypes.BlockData{
					Round:   5,
					Payload: &drctypes.Payload{},
					Qc: &drctypes.QuorumCert{
						VoteInfo: r4vInfo,
						LedgerCommitInfo: &types.UnicitySeal{Version: 1,
							PreviousHash: r4vInfo.Hash(gocrypto.SHA256),
							Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
						},
						Signatures: map[string][]byte{"test": test.RandomBytes(65)},
					},
				},
				Qc: &drctypes.QuorumCert{
					VoteInfo: r5vInfo,
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: r5vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
				CommitQc: &drctypes.QuorumCert{
					VoteInfo: r6vInfo,
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: r6vInfo.Hash(gocrypto.SHA256),
						Signatures:   map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			},
			BlockData: []*drctypes.BlockData{{
				Round:   6,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: r5vInfo,
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash:         r5vInfo.Hash(gocrypto.SHA256),
						RootChainRoundNumber: 5,
						Hash:                 test.RandomBytes(32),
						Signatures:           map[string][]byte{"test": test.RandomBytes(65)},
					},
					Signatures: map[string][]byte{"test": test.RandomBytes(65)},
				},
			},
			},
		}
		tb := testtb.NewTrustBase(t, AlwaysValidVerifier{})
		require.NoError(t, sm.Verify(gocrypto.SHA256, tb))
	})
}

func TestRecoveryBlock_IsValid(t *testing.T) {
	t.Run("input record state is invalid", func(t *testing.T) {
		r := &CommittedBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         types.NewTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: nil,
		}
		require.ErrorContains(t, r.IsValid(), "missing input record state")
	})
	t.Run("input record state is nil", func(t *testing.T) {
		r := &CommittedBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         types.NewTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					Partition: 1,
					Ir:        nil,
					PDRHash:   make([]byte, 32),
				},
			},
		}
		require.ErrorContains(t, r.IsValid(), "invalid input record: input record is nil")
	})
	t.Run("block data is nil", func(t *testing.T) {
		r := &CommittedBlock{
			Block: nil,
			Ir: []*InputData{
				{
					Partition: 1,
					Ir: &types.InputRecord{Version: 1,
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					PDRHash: make([]byte, 32),
				},
			},
		}
		require.ErrorContains(t, r.IsValid(), "block data is nil")
	})
	t.Run("block data is invalid", func(t *testing.T) {
		r := &CommittedBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:     4,
						CurrentRootHash: test.RandomBytes(32),
						Timestamp:       types.NewTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					Partition: 1,
					Ir: &types.InputRecord{Version: 1,
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					PDRHash: make([]byte, 32),
				},
			},
		}
		require.ErrorContains(t, r.IsValid(), "block data error: invalid quorum certificate: invalid vote info: parent round number is not assigned")
	})
	t.Run("head is missing qc", func(t *testing.T) {
		r := &CommittedBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         types.NewTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					Partition: 1,
					Ir: &types.InputRecord{Version: 1,
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					PDRHash: make([]byte, 32),
				},
			},
		}
		require.ErrorContains(t, r.IsValid(), "commit head is missing qc certificate")
	})
	t.Run("head is missing commit qc", func(t *testing.T) {
		r := &CommittedBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         types.NewTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					Partition: 1,
					Ir: &types.InputRecord{Version: 1,
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					PDRHash: make([]byte, 32),
				},
			},
			Qc: &drctypes.QuorumCert{},
		}
		require.ErrorContains(t, r.IsValid(), "commit head is missing commit qc certificate")
	})
	t.Run("ok", func(t *testing.T) {
		r := &CommittedBlock{
			Block: &drctypes.BlockData{
				Round:   5,
				Payload: &drctypes.Payload{},
				Qc: &drctypes.QuorumCert{
					VoteInfo: &drctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         types.NewTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{Version: 1,
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			Ir: []*InputData{
				{
					Partition: 1,
					Ir: &types.InputRecord{Version: 1,
						PreviousHash:    test.RandomBytes(32),
						Hash:            test.RandomBytes(32),
						BlockHash:       test.RandomBytes(32),
						SummaryValue:    test.RandomBytes(32),
						RoundNumber:     3,
						SumOfEarnedFees: 10,
					},
					PDRHash: make([]byte, 32),
				},
			},
			Qc:       &drctypes.QuorumCert{},
			CommitQc: &drctypes.QuorumCert{},
		}
		require.NoError(t, r.IsValid())
	})
}

func TestInputData_IsValid(t *testing.T) {
	t.Run("input record is nil", func(t *testing.T) {
		i := &InputData{
			Partition: 0,
			Ir:        nil,
			PDRHash:   nil,
		}
		require.ErrorContains(t, i.IsValid(), "input record is nil")
	})
	t.Run("system description hash is not set", func(t *testing.T) {
		i := &InputData{
			Partition: 0,
			Ir: &types.InputRecord{Version: 1,
				PreviousHash:    test.RandomBytes(32),
				Hash:            test.RandomBytes(32),
				BlockHash:       test.RandomBytes(32),
				SummaryValue:    test.RandomBytes(32),
				RoundNumber:     3,
				SumOfEarnedFees: 10,
			},
			PDRHash: nil,
		}
		require.ErrorContains(t, i.IsValid(), "system description hash not set")
	})
	t.Run("ok", func(t *testing.T) {
		i := &InputData{
			Partition: 0,
			Ir: &types.InputRecord{Version: 1,
				PreviousHash:    test.RandomBytes(32),
				Hash:            test.RandomBytes(32),
				BlockHash:       test.RandomBytes(32),
				SummaryValue:    test.RandomBytes(32),
				RoundNumber:     3,
				SumOfEarnedFees: 10,
			},
			PDRHash: test.RandomBytes(32),
		}
		require.NoError(t, i.IsValid())
	})
}
