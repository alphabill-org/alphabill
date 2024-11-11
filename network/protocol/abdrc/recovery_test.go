package abdrc

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/crypto/canonicalizer"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
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

func (a AlwaysValidVerifier) UnmarshalPubKey() (crypto.PublicKey, error) {
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
			Block: &rctypes.BlockData{
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
				Block: &rctypes.BlockData{
					Round: 5,
					Qc:    &rctypes.QuorumCert{VoteInfo: &rctypes.RoundInfo{RoundNumber: 4}},
				},
			},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(3), "can't recover to round 3 with committed block for round 5")
	})
	t.Run("exact block for round not found", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{
				Block: &rctypes.BlockData{
					Round: 5,
					Qc:    &rctypes.QuorumCert{VoteInfo: &rctypes.RoundInfo{RoundNumber: 4}},
				},
			},
		}
		require.ErrorContains(t, sm.CanRecoverToRound(8), "state has no data block for round 8")
	})
	t.Run("commit head is the block needed", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{
				Block: &rctypes.BlockData{
					Round: 5,
					Qc:    &rctypes.QuorumCert{VoteInfo: &rctypes.RoundInfo{RoundNumber: 4}},
				},
			},
		}
		require.NoError(t, sm.CanRecoverToRound(5))
	})
	t.Run("most common case", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: &CommittedBlock{
				Block: &rctypes.BlockData{
					Round: 5,
					Qc:    &rctypes.QuorumCert{VoteInfo: &rctypes.RoundInfo{RoundNumber: 4}},
				},
			},
			BlockData: []*rctypes.BlockData{
				{
					Round: 6,
					Qc:    &rctypes.QuorumCert{VoteInfo: &rctypes.RoundInfo{RoundNumber: 5}},
				},
				{
					Round: 7,
					Qc:    &rctypes.QuorumCert{VoteInfo: &rctypes.RoundInfo{RoundNumber: 6}},
				},
			},
		}
		require.NoError(t, sm.CanRecoverToRound(7))
	})
}

func TestStateMsg_Verify(t *testing.T) {
	r4vInfo := &rctypes.RoundInfo{
		RoundNumber:       4,
		ParentRoundNumber: 3,
		CurrentRootHash:   test.RandomBytes(32),
		Timestamp:         types.NewTimestamp(),
	}
	r5vInfo := &rctypes.RoundInfo{
		RoundNumber:       5,
		ParentRoundNumber: 4,
		CurrentRootHash:   test.RandomBytes(32),
		Timestamp:         types.NewTimestamp(),
	}
	r6vInfo := &rctypes.RoundInfo{
		RoundNumber:       6,
		ParentRoundNumber: 5,
		CurrentRootHash:   test.RandomBytes(32),
		Timestamp:         types.NewTimestamp(),
	}

	headIR := &types.InputRecord{
		Version:         1,
		PreviousHash:    test.RandomBytes(32),
		Hash:            test.RandomBytes(32),
		BlockHash:       test.RandomBytes(32),
		SummaryValue:    test.RandomBytes(32),
		RoundNumber:     3,
		SumOfEarnedFees: 10,
	}

	pdr := types.PartitionDescriptionRecord{
		PartitionIdentifier: 1,
	}

	signer, _ := testsig.CreateSignerAndVerifier(t)

	uc := testcertificates.CreateUnicityCertificate(
		t,
		signer,
		headIR,
		&pdr,
		1,
		make([]byte, 32),
	)

	validStateMsg := func() StateMsg {
		return StateMsg{
			CommittedHead: &CommittedBlock{
				ShardInfo: []ShardInfo{{
					Partition:     1,
					Round:         5,
					PrevEpochStat: []byte{0, 0, 0, 0, 0},
					PrevEpochFees: []byte{0xF, 0xE, 0xE, 5},
					RootHash:      test.RandomBytes(32),
					Fees:          map[string]uint64{"A": 0},
					Leader:        "leader id",
					UC:            *uc,
					IR:            headIR,
					PDRHash:       pdr.Hash(crypto.SHA256),
				}},
				Block: &rctypes.BlockData{
					Round:   5,
					Payload: &rctypes.Payload{},
					Qc: &rctypes.QuorumCert{
						VoteInfo: r4vInfo,
						LedgerCommitInfo: &types.UnicitySeal{
							Version:      1,
							PreviousHash: r4vInfo.Hash(crypto.SHA256),
							Signatures:   map[string]hex.Bytes{"test": test.RandomBytes(65)},
						},
						Signatures: map[string]hex.Bytes{"test": test.RandomBytes(65)},
					},
				},
				Qc: &rctypes.QuorumCert{
					VoteInfo: r5vInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						Version:      1,
						PreviousHash: r5vInfo.Hash(crypto.SHA256),
						Signatures:   map[string]hex.Bytes{"test": test.RandomBytes(65)},
					},
					Signatures: map[string]hex.Bytes{"test": test.RandomBytes(65)},
				},
				CommitQc: &rctypes.QuorumCert{
					VoteInfo: r6vInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						Version:      1,
						PreviousHash: r6vInfo.Hash(crypto.SHA256),
						Signatures:   map[string]hex.Bytes{"test": test.RandomBytes(65)},
					},
					Signatures: map[string]hex.Bytes{"test": test.RandomBytes(65)},
				},
			},
			BlockData: []*rctypes.BlockData{{
				Round:   6,
				Payload: &rctypes.Payload{},
				Qc: &rctypes.QuorumCert{
					VoteInfo: r5vInfo,
					LedgerCommitInfo: &types.UnicitySeal{
						Version:              1,
						PreviousHash:         r5vInfo.Hash(crypto.SHA256),
						RootChainRoundNumber: 5,
						Hash:                 test.RandomBytes(32),
						Signatures:           map[string]hex.Bytes{"test": test.RandomBytes(65)},
					},
					Signatures: map[string]hex.Bytes{"test": test.RandomBytes(65)},
				},
			},
			},
		}
	}

	t.Run("ok", func(t *testing.T) {
		sm := validStateMsg()
		tb := testtb.NewTrustBase(t, AlwaysValidVerifier{})
		require.NoError(t, sm.Verify(crypto.SHA256, tb))
	})

	t.Run("commit head is nil", func(t *testing.T) {
		sm := &StateMsg{
			CommittedHead: nil,
			BlockData:     nil,
		}
		require.ErrorContains(t, sm.Verify(crypto.SHA256, &types.RootTrustBaseV1{}), "commit head is nil")
	})

	t.Run("commit head, invalid block", func(t *testing.T) {
		sm := validStateMsg()
		sm.CommittedHead.Block.Qc = nil
		require.ErrorContains(t, sm.Verify(crypto.SHA256, &types.RootTrustBaseV1{}), "invalid commit head: invalid block data: proposed block is missing quorum certificate")
	})

	t.Run("commit head, invalid QC", func(t *testing.T) {
		sm := validStateMsg()
		sm.CommittedHead.Block.Qc.LedgerCommitInfo.PreviousHash[0]++
		require.EqualError(t, sm.Verify(crypto.SHA256, &types.RootTrustBaseV1{}), "block qc verification error: vote info hash verification failed")
	})

	t.Run("invalid block node data", func(t *testing.T) {
		sm := validStateMsg()
		sm.BlockData[0].Qc = nil
		tb := testtb.NewTrustBase(t, AlwaysValidVerifier{})
		require.ErrorContains(t, sm.Verify(crypto.SHA256, tb), "invalid block node: proposed block is missing quorum certificate")
	})
}

func TestRecoveryBlock_IsValid(t *testing.T) {
	headIR := &types.InputRecord{
		Version:         1,
		PreviousHash:    test.RandomBytes(32),
		Hash:            test.RandomBytes(32),
		BlockHash:       test.RandomBytes(32),
		SummaryValue:    test.RandomBytes(32),
		RoundNumber:     3,
		SumOfEarnedFees: 10,
	}

	pdr := types.PartitionDescriptionRecord{
		PartitionIdentifier: 1,
	}

	signer, _ := testsig.CreateSignerAndVerifier(t)

	uc := testcertificates.CreateUnicityCertificate(
		t,
		signer,
		headIR,
		&pdr,
		1,
		make([]byte, 32),
	)
	validBlock := func() CommittedBlock {
		return CommittedBlock{
			Block: &rctypes.BlockData{
				Round:   5,
				Payload: &rctypes.Payload{},
				Qc: &rctypes.QuorumCert{
					VoteInfo: &rctypes.RoundInfo{
						RoundNumber:       4,
						ParentRoundNumber: 3,
						CurrentRootHash:   test.RandomBytes(32),
						Timestamp:         types.NewTimestamp(),
					},
					LedgerCommitInfo: &types.UnicitySeal{
						Version:      1,
						PreviousHash: test.RandomBytes(32),
					},
				},
			},
			ShardInfo: []ShardInfo{{
				Partition:     1,
				Round:         5,
				PrevEpochStat: []byte{0, 0, 0, 0, 0},
				PrevEpochFees: []byte{0xF, 0xE, 0xE, 5},
				RootHash:      test.RandomBytes(32),
				Fees:          map[string]uint64{"A": 10},
				Leader:        "777",
				UC:            *uc,
				IR:            headIR,
				PDRHash:       pdr.Hash(crypto.SHA256),
			}},
			Qc:       &rctypes.QuorumCert{},
			CommitQc: &rctypes.QuorumCert{},
		}
	}

	b := validBlock()
	require.NoError(t, b.IsValid())

	t.Run("input record state is invalid", func(t *testing.T) {
		r := validBlock()
		r.ShardInfo = nil
		require.ErrorContains(t, r.IsValid(), "missing ShardInfo")
	})

	t.Run("input record state is nil", func(t *testing.T) {
		r := validBlock()
		r.ShardInfo[0].IR = nil
		require.ErrorContains(t, r.IsValid(), "invalid ShardInfo[00000001 - ]: invalid input record: input record is nil")
	})

	t.Run("block data is nil", func(t *testing.T) {
		r := validBlock()
		r.Block = nil
		require.ErrorContains(t, r.IsValid(), "block data is nil")
	})

	t.Run("block data is invalid", func(t *testing.T) {
		r := validBlock()
		r.Block.Qc.VoteInfo.ParentRoundNumber = 0
		require.ErrorContains(t, r.IsValid(), "invalid block data: invalid quorum certificate: invalid vote info: parent round number is not assigned")
	})

	t.Run("head is missing qc", func(t *testing.T) {
		r := validBlock()
		r.Qc = nil
		require.ErrorContains(t, r.IsValid(), "commit head is missing qc certificate")
	})

	t.Run("head is missing commit qc", func(t *testing.T) {
		r := validBlock()
		r.CommitQc = nil
		require.ErrorContains(t, r.IsValid(), "commit head is missing commit qc certificate")
	})
}
