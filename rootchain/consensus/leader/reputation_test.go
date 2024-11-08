package leader

import (
	"fmt"
	mrand "math/rand"
	"testing"

	abt "github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	test "github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	"github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func Test_ReputationBased_Update(t *testing.T) {
	t.Parallel()

	t.Run("invalid input: qc.ParentRound + 1 != qc.Round", func(t *testing.T) {
		rl := &ReputationBased{}
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) { return nil, fmt.Errorf("should not get this far") }
		slotIdx := rl.curIdx

		err := rl.Update(&types.QuorumCert{VoteInfo: &types.RoundInfo{RoundNumber: 5, ParentRoundNumber: 3}}, 6, loadBlock)
		require.EqualError(t, err, `not updating leaders because rounds are not consecutive {parent: 3, QC: 5, current: 6}`)
		require.Equal(t, slotIdx, rl.curIdx, "expected that round leader buffer index is not changed")

		err = rl.Update(&types.QuorumCert{VoteInfo: &types.RoundInfo{RoundNumber: 1, ParentRoundNumber: 1}}, 2, loadBlock)
		require.EqualError(t, err, `not updating leaders because rounds are not consecutive {parent: 1, QC: 1, current: 2}`)
		require.Equal(t, slotIdx, rl.curIdx, "expected that round leader buffer index is not changed")

		err = rl.Update(&types.QuorumCert{VoteInfo: &types.RoundInfo{RoundNumber: 1, ParentRoundNumber: 2}}, 2, loadBlock)
		require.EqualError(t, err, `not updating leaders because rounds are not consecutive {parent: 2, QC: 1, current: 2}`)
		require.Equal(t, slotIdx, rl.curIdx, "expected that round leader buffer index is not changed")
	})

	t.Run("invalid input: qc.Round + 1 != currentRound", func(t *testing.T) {
		rl := &ReputationBased{}
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) { return nil, fmt.Errorf("should not get this far") }
		slotIdx := rl.curIdx

		err := rl.Update(&types.QuorumCert{VoteInfo: &types.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1}}, 1, loadBlock)
		require.EqualError(t, err, `not updating leaders because rounds are not consecutive {parent: 1, QC: 2, current: 1}`)
		require.Equal(t, slotIdx, rl.curIdx, "expected that round leader buffer index is not changed")

		err = rl.Update(&types.QuorumCert{VoteInfo: &types.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1}}, 2, loadBlock)
		require.EqualError(t, err, `not updating leaders because rounds are not consecutive {parent: 1, QC: 2, current: 2}`)
		require.Equal(t, slotIdx, rl.curIdx, "expected that round leader buffer index is not changed")

		err = rl.Update(&types.QuorumCert{VoteInfo: &types.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1}}, 4, loadBlock)
		require.EqualError(t, err, `not updating leaders because rounds are not consecutive {parent: 1, QC: 2, current: 4}`)
		require.Equal(t, slotIdx, rl.curIdx, "expected that round leader buffer index is not changed")
	})

	t.Run("failing to elect leader because block loader fails", func(t *testing.T) {
		expErr := fmt.Errorf("no blocks for you")
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) { return nil, expErr }
		rl := &ReputationBased{
			windowSize: 1,
		}
		slotIdx := rl.curIdx

		qc := &types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1},
		}
		err := rl.Update(qc, 3, loadBlock)
		require.ErrorIs(t, err, expErr)
		require.Equal(t, slotIdx, rl.curIdx, "expected that round leader buffer index is not changed")
	})

	// valid signer id-s to use in following tests
	peerIDs := test.GeneratePeerIDs(t, 2)
	signerAid, signerAkey := peerIDs[0], peerIDs[0].String()
	signerBid, signerBkey := peerIDs[1], peerIDs[1].String()

	t.Run("successfully electing leader", func(t *testing.T) {
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) {
			if round != 1 {
				return nil, fmt.Errorf("expected that round 1 block is requested, got request for round %d", round)
			}
			return &storage.ExecutedBlock{BlockData: &types.BlockData{Author: signerAkey}}, nil
		}
		rl, err := NewReputationBased([]peer.ID{signerAid, signerBid}, 1, 1)
		require.NoError(t, err)
		require.NotNil(t, rl)

		qc := &types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1},
			Signatures:       map[string]hex.Bytes{signerAkey: {1, 2, 3}, signerBkey: {4, 5, 6}},
		}
		err = rl.Update(qc, 3, loadBlock)
		require.NoError(t, err)
		// "signer A" is the author of the previous "interesting block" (1) so "signer B" must
		// have been elected to be the leader of the next round (4)
		require.Equal(t, signerBid, rl.leaders[rl.curIdx].leader)
		require.EqualValues(t, 4, rl.leaders[rl.curIdx].round)
	})

	t.Run("same input twice in a row", func(t *testing.T) {
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) {
			if round != 1 {
				return nil, fmt.Errorf("expected that round 1 block is requested, got request for round %d", round)
			}
			return &storage.ExecutedBlock{BlockData: &types.BlockData{Author: signerAkey}}, nil
		}
		rl, err := NewReputationBased([]peer.ID{signerAid, signerBid}, 1, 1)
		require.NoError(t, err)
		require.NotNil(t, rl)

		qc := &types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{RoundNumber: 2, ParentRoundNumber: 1},
			Signatures:       map[string]hex.Bytes{signerAkey: {1, 2, 3}, signerBkey: {4, 5, 6}},
		}
		require.NoError(t, rl.Update(qc, 3, loadBlock))
		// "signer A" is the author of the previous block so "signer B" must have been elected
		// to be the leader of the next round (4)
		require.Equal(t, signerBid, rl.leaders[rl.curIdx].leader)
		require.EqualValues(t, 4, rl.leaders[rl.curIdx].round)
		curIdx := rl.curIdx

		// repeat the update call with the same input - should not cause any changes
		require.NoError(t, rl.Update(qc, 3, loadBlock))
		require.Equal(t, curIdx, rl.curIdx, "expected that slot index for the round doesn't change")
		require.Equal(t, signerBid, rl.leaders[rl.curIdx].leader)
		require.EqualValues(t, 4, rl.leaders[rl.curIdx].round)

		// invalid input (jump current round to 5 with the same QC) - shouldn't change state
		err = rl.Update(qc, 5, loadBlock)
		require.EqualError(t, err, `not updating leaders because rounds are not consecutive {parent: 1, QC: 2, current: 5}`)
		require.Equal(t, curIdx, rl.curIdx, "expected that slot index for the round doesn't change")
		require.Equal(t, signerBid, rl.leaders[rl.curIdx].leader)
		require.EqualValues(t, 4, rl.leaders[rl.curIdx].round)
	})
}

func Test_ReputationBased_GetLeaderForRound(t *testing.T) {
	t.Parallel()

	t.Run("no elected leaders, fallback to round-robin", func(t *testing.T) {
		ids := test.GeneratePeerIDs(t, 10)

		rl, err := NewReputationBased(ids, 1, 1)
		require.NoError(t, err)
		require.NotNil(t, rl)

		// make len(ids) iterations and on each loop ask len(ids) leaders - so we should
		// start from different starting round (1, 2,...) each time we start collecting
		// but in the end we should have each ID only once in roundLeaders slice
		for i := 0; i < len(ids); i++ {
			roundLeaders := []peer.ID{}
			for round := i + 1; len(roundLeaders) < len(ids); round++ {
				roundLeaders = append(roundLeaders, rl.GetLeaderForRound(uint64(round)))
			}
			// elements mustn't repeat so should have the slice with the same IDs we passed to algorithm
			require.ElementsMatch(t, roundLeaders, ids, "iteration %d", i)
		}
	})

	t.Run("reading leader from elected leaders slice", func(t *testing.T) {
		ids := test.GeneratePeerIDs(t, 4)
		signerAid, signerBid, signerCid, signerDid := ids[0], ids[1], ids[2], ids[3]

		rl, err := NewReputationBased(ids, 1, 1)
		require.NoError(t, err)
		require.NotNil(t, rl)
		require.Len(t, rl.leaders, 3, "leaders buffer len has changed, this test needs to be updated accordingly")

		idx := rl.slotIndex(23)
		rl.leaders[idx].round = 23
		rl.leaders[idx].leader = signerAid
		require.Equal(t, signerAid, rl.GetLeaderForRound(23))

		idx = rl.slotIndex(24)
		rl.leaders[idx].round = 24
		rl.leaders[idx].leader = signerBid
		require.Equal(t, signerBid, rl.GetLeaderForRound(24))
		require.Equal(t, signerAid, rl.GetLeaderForRound(23))

		idx = rl.slotIndex(26)
		rl.leaders[idx].round = 26
		rl.leaders[idx].leader = signerCid
		require.Equal(t, signerCid, rl.GetLeaderForRound(26))
		require.Equal(t, signerBid, rl.GetLeaderForRound(24))
		require.Equal(t, signerAid, rl.GetLeaderForRound(23))

		// we do have 3 slots so with the next call we should have replaced the
		// signer A and round 23
		idx = rl.slotIndex(27)
		rl.leaders[idx].round = 27
		rl.leaders[idx].leader = signerDid
		require.Equal(t, signerDid, rl.GetLeaderForRound(27))
		require.Equal(t, signerCid, rl.GetLeaderForRound(26))
		require.Equal(t, signerBid, rl.GetLeaderForRound(24))
		for i, v := range rl.leaders {
			if v.round == 23 {
				t.Errorf("unexpectedly round 23 is still in leaders cache: [%d]=%#v", i, v)
			}
		}
	})
}

func Test_ReputationBased_electLeader(t *testing.T) {
	t.Parallel()

	t.Run("loading block fails with error", func(t *testing.T) {
		expErr := fmt.Errorf("failed to load the block")
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) { return nil, expErr }
		rl := &ReputationBased{
			windowSize:  1,
			excludeSize: 1,
		}
		id, err := rl.electLeader(&types.QuorumCert{LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}}}, loadBlock)
		require.ErrorIs(t, err, expErr)
		require.EqualValues(t, UnknownLeader, id)
	})

	t.Run("loading block fails - nil block is returned", func(t *testing.T) {
		t.Skip("crashes")
		// this is unexpected behavior - we define that blockLoader must not return nil
		// block and nil error. current implementation survives this (ie doesn't crash)
		// and because it wasn't able to get any info about block signers we get error
		// about no active validators
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) { return nil, nil }
		rl := &ReputationBased{
			windowSize:  3,
			excludeSize: 1,
		}
		id, err := rl.electLeader(&types.QuorumCert{VoteInfo: &types.RoundInfo{ParentRoundNumber: 3}}, loadBlock)
		require.EqualError(t, err, `no active validators left after eliminating 1 recent authors`)
		require.EqualValues(t, UnknownLeader, id)
	})

	t.Run("single signer will be excluded and thus empty set to select from", func(t *testing.T) {
		loadBlock := func(round uint64) (*storage.ExecutedBlock, error) {
			return &storage.ExecutedBlock{BlockData: &types.BlockData{Author: "signer"}}, nil
		}
		rl := &ReputationBased{
			windowSize:  1,
			excludeSize: 1,
		}
		id, err := rl.electLeader(&types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{ParentRoundNumber: 3},
			Signatures:       map[string]hex.Bytes{"signer": {1, 2, 3}},
		}, loadBlock)
		require.EqualError(t, err, `no active validators left after eliminating 1 recent authors`)
		require.EqualValues(t, UnknownLeader, id)
	})

	// valid signer id-s to use in the following unit tests
	peerIDs := test.GeneratePeerIDs(t, 3)

	t.Run("no authors excluded (excludeSize=0)", func(t *testing.T) {
		signerAid, signerAkey := peerIDs[0], peerIDs[0].String()
		rl := &ReputationBased{
			windowSize:  1,
			excludeSize: 0,
		}
		id, err := rl.electLeader(&types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{ParentRoundNumber: 3},
			Signatures:       map[string]hex.Bytes{signerAkey: {1, 2, 3}},
		}, func(round uint64) (*storage.ExecutedBlock, error) {
			return &storage.ExecutedBlock{BlockData: &types.BlockData{Author: signerAkey}}, nil
		})
		require.NoError(t, err)
		require.EqualValues(t, signerAid, id)
	})

	t.Run("elected signer id is in invalid encoding", func(t *testing.T) {
		signerAkey := peerIDs[0].String()
		rl := &ReputationBased{
			windowSize:  1,
			excludeSize: 1,
		}
		id, err := rl.electLeader(&types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{ParentRoundNumber: 3},
			Signatures:       map[string]hex.Bytes{signerAkey: {1, 2, 3}, "oh deer": {4, 5, 6}},
		}, func(round uint64) (*storage.ExecutedBlock, error) {
			return &storage.ExecutedBlock{BlockData: &types.BlockData{Author: signerAkey}}, nil
		})
		// signer A is the author of the previous block so the other signer must have been elected
		// to be the leader but it has invalid ID
		require.EqualError(t, err, `invalid peer id "oh deer": failed to parse peer ID: invalid cid: selected encoding not supported`)
		require.EqualValues(t, UnknownLeader, id)
	})

	t.Run("two active validators", func(t *testing.T) {
		signerAkey := peerIDs[0].String()
		signerBkey, signerBid := peerIDs[1].String(), peerIDs[1]
		rl := &ReputationBased{
			windowSize:  1,
			excludeSize: 1,
		}
		id, err := rl.electLeader(&types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{ParentRoundNumber: 3},
			Signatures:       map[string]hex.Bytes{signerAkey: {1, 2, 3}, signerBkey: {4, 5, 6}},
		}, func(round uint64) (*storage.ExecutedBlock, error) {
			return &storage.ExecutedBlock{BlockData: &types.BlockData{Author: signerAkey}}, nil
		})
		// "signer A" is the author of the previous block so "signer B" must have been elected to be the leader
		require.NoError(t, err)
		require.Equal(t, signerBid, id)
	})

	t.Run("exclude two last authors", func(t *testing.T) {
		signerAkey := peerIDs[0].String()
		signerBkey := peerIDs[1].String()
		signerCkey, signerCid := peerIDs[2].String(), peerIDs[2]
		rl := &ReputationBased{
			windowSize:  1,
			excludeSize: 2,
		}
		id, err := rl.electLeader(&types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{RoundNumber: 5, ParentRoundNumber: 4},
			Signatures:       map[string]hex.Bytes{signerAkey: {1, 2, 3}, signerBkey: {4, 5, 6}, signerCkey: {7, 8, 9}},
		}, func(round uint64) (*storage.ExecutedBlock, error) {
			switch round {
			case 3:
				return &storage.ExecutedBlock{BlockData: &types.BlockData{Author: signerAkey}}, nil
			case 4:
				return &storage.ExecutedBlock{
					BlockData: &types.BlockData{
						Author: signerBkey,
						Qc: &types.QuorumCert{
							VoteInfo: &types.RoundInfo{RoundNumber: 3},
						},
					},
				}, nil
			}
			return nil, fmt.Errorf("no block data for round %d", round)
		})
		// signer A and B are the authors of the previous two block so C must have been elected to be the leader
		require.NoError(t, err)
		require.Equal(t, signerCid, id)
	})
}

func Test_ReputationBased_roundIndex(t *testing.T) {
	t.Parallel()

	rl := &ReputationBased{}

	getRoundIndex := func(round uint64) int {
		// mimic what happens in actual use case ie save round into the idx slot
		idx := rl.slotIndex(round)
		rl.leaders[idx].round = round
		rl.leaders[idx].leader = peer.ID(fmt.Sprintf("round %d leader", round))
		return idx
	}

	// different rounds do get different index
	idx1 := getRoundIndex(43)
	idx2 := getRoundIndex(44)
	require.NotEqual(t, idx1, idx2)
	// asking for the same round as last time should return the same index
	idx1 = getRoundIndex(44)
	require.Equal(t, idx1, idx2)

	// different round, smaller than the previous one, expect new index
	// ie leader election doesn't validate that rounds move only in one
	// direction (increase) that's the callers job
	idx2 = getRoundIndex(40)
	require.NotEqual(t, idx1, idx2)
	// jump forward (round numbers do not have to be consecutive)
	idx1 = getRoundIndex(60)
	require.NotEqual(t, idx1, idx2)
	require.Equal(t, idx1, getRoundIndex(60))

	// asking r, r+1, r returns 3 different indices
	idx1 = getRoundIndex(61)
	idx2 = getRoundIndex(62)
	idx3 := getRoundIndex(61)
	require.NotEqual(t, idx1, idx2)
	require.NotEqual(t, idx1, idx3)
	require.NotEqual(t, idx2, idx3)
}

func Test_NewReputationBased(t *testing.T) {
	t.Parallel()

	t.Run("must provide validators", func(t *testing.T) {
		ls, err := NewReputationBased(nil, 1, 1)
		require.EqualError(t, err, `peer list (validators) must not be empty`)
		require.Nil(t, ls)

		ls, err = NewReputationBased([]peer.ID{}, 1, 1)
		require.EqualError(t, err, `peer list (validators) must not be empty`)
		require.Nil(t, ls)
	})

	t.Run("len(validators) == excludeSize", func(t *testing.T) {
		ls, err := NewReputationBased([]peer.ID{"A"}, 1, 1)
		require.EqualError(t, err, `excludeSize value must be smaller than the number of validators in the system (1 validators, exclude 1)`)
		require.Nil(t, ls)
	})

	t.Run("len(validators) < excludeSize", func(t *testing.T) {
		ls, err := NewReputationBased([]peer.ID{"A"}, 1, 2)
		require.EqualError(t, err, `excludeSize value must be smaller than the number of validators in the system (1 validators, exclude 2)`)
		require.Nil(t, ls)
	})

	t.Run("invalid windowSize: 0", func(t *testing.T) {
		lr, err := NewReputationBased([]peer.ID{"A", "B"}, 0, 1)
		require.EqualError(t, err, `window size must be greater than zero`)
		require.Nil(t, lr)
	})

	t.Run("success", func(t *testing.T) {
		peerIDs := []peer.ID{"A", "B", "C"}
		ls, err := NewReputationBased(peerIDs, 1, 2)
		require.NoError(t, err)
		require.NotNil(t, ls)
		require.ElementsMatch(t, ls.validators, peerIDs)
		require.Equal(t, 1, ls.windowSize)
		require.Equal(t, 2, ls.excludeSize)
	})
}

func Test_ReputationBased(t *testing.T) {
	t.Parallel()

	// generate some peer IDs
	peerIDs := test.GeneratePeerIDs(t, 10)
	// we mostly need ID's string representation so as a optimization convert once here
	peerIDstr := make([]string, len(peerIDs))
	for k, v := range peerIDs {
		peerIDstr[k] = v.String()
	}

	// mock block store, maps roundNumber to block of the round
	blockStore := map[uint64]*storage.ExecutedBlock{}
	// to save the block to blockStore
	storeBlock := func(bd *storage.ExecutedBlock) error {
		if _, ok := blockStore[bd.BlockData.Round]; ok {
			return fmt.Errorf("round %d block already stored", bd.BlockData.Round)
		}
		blockStore[bd.BlockData.Round] = bd
		return nil
	}
	// load block of given round from blockStore
	loadBlock := func(round uint64) (*storage.ExecutedBlock, error) {
		if b, ok := blockStore[round]; ok {
			return b, nil
		}
		return nil, fmt.Errorf("no block for round %d", round)
	}

	dumpBlock := func(round uint64) {
		b, err := loadBlock(round)
		if err != nil {
			t.Logf("failed to load round %d block: %v", round, err)
			return
		}
		sign := []string{}
		for k := range b.BlockData.Qc.Signatures {
			sign = append(sign, k)
		}
		t.Logf("block[%d] Author: %s Signed (%d): %v", round, b.BlockData.Author, len(sign), sign)
	}

	// processRound generates block for the given round and adds it to block store.
	// "rl" is used to get leader for the round (block author) and it's Update
	// method is called with the QC of the new block
	// "round" is supposed to be the current round as per pacemaker, must be >= 2
	// so basically it mimics one full cycle in consensus manager (collecting votes for
	// the round, getting quorum, adding block to the tree...)
	processRound := func(rl *ReputationBased, round uint64) error {
		// create QC for the previous round
		qc := &types.QuorumCert{
			LedgerCommitInfo: &abt.UnicitySeal{Version: 1, PreviousHash: []byte{0, 0, 0, 0}},
			VoteInfo:         &types.RoundInfo{RoundNumber: round - 1, ParentRoundNumber: round - 2},
			Signatures:       make(map[string]hex.Bytes),
		}
		// add n-f random signers, select f randomly in range [0..4)
		cnt := len(peerIDs) - mrand.Intn(4)
		for _, v := range mrand.Perm(len(peerIDs))[:cnt] {
			qc.Signatures[peerIDstr[v]] = []byte{byte(v & 0xFF)}
		}
		// leader for this round has been elected by previous round
		leaderID := rl.GetLeaderForRound(round)
		require.NoError(t, storeBlock(&storage.ExecutedBlock{BlockData: &types.BlockData{
			Round:  round,
			Author: leaderID.String(),
			Qc:     qc,
		}}))
		// elect leader for round "round+1"
		return rl.Update(qc, round, loadBlock)
	}

	// *** TESTS ***

	// create election with windowSize=len(peerIDs) - as our block store is empty
	// loading blocks should fail and we fall back to round-robin selection for
	// len(peerIDs) rounds
	rl, err := NewReputationBased(peerIDs, len(peerIDs), 1)
	require.NoError(t, err)

	var currentRound uint64 = 1 // genesis round is 1
	for i := 0; i < len(peerIDs); i++ {
		currentRound++
		err := processRound(rl, currentRound)
		require.Error(t, err)
		require.Contains(t, err.Error(), fmt.Sprintf("failed to elect leader for round %d:", currentRound+1))
	}
	// validate: blockStore must now contain len(peerIDs) blocks (IOW block per author)
	// starting with round 2, authors in round-robin order
	for k := range peerIDstr {
		round := uint64(k + 2)
		idx := round % uint64(len(peerIDstr)) // round-robin index
		require.Equal(t, peerIDstr[idx], blockStore[round].BlockData.Author, "unexpected author for round %d, index %d", round, idx)
	}

	// when processing round R (ie we create block for that round) we will
	// elect leader for round R+1 excluding authors of the blocks [R-2 .. R-2-ExS]
	// so the block author may not be the same in that range.
	checkAuthor := func(rl *ReputationBased, round uint64) {
		a := blockStore[round].BlockData.Author
		for i := 0; i < rl.excludeSize; i++ {
			r := round - 3 - uint64(i)
			if a == blockStore[r].BlockData.Author {
				t.Errorf("rounds %d and %d have same author %s, author shouldn't repeat in the range [%d .. %d]", round, r, a, round-3, round-2-uint64(rl.excludeSize))
				for i := 0; i < rl.excludeSize; i++ {
					r := round - 3 - uint64(i)
					dumpBlock(r)
				}
				return
			}
		}
	}

	// change election parameters and generate some blocks
	rl.windowSize = 3
	rl.excludeSize = 3
	for i := 0; i < 2*len(peerIDs); i++ {
		currentRound++
		require.NoError(t, processRound(rl, currentRound))
	}
	// as leader for first blocks were generated by round-robin they must be unique and thus
	// also not trigger author check here. we must start at least round 4 as otherwise we
	// try to load block for round < 2
	for i := 4 + rl.excludeSize; i <= int(currentRound); i++ {
		round := uint64(i)
		checkAuthor(rl, round)
	}

	// change params - smaller window but exclude more than the window!
	rl.windowSize = 1
	rl.excludeSize = 3
	startIndex := currentRound
	for i := 0; i < 2*len(peerIDs); i++ {
		currentRound++
		require.NoError(t, processRound(rl, currentRound))
	}
	for i := int(startIndex); i <= int(currentRound); i++ {
		checkAuthor(rl, uint64(i))
	}

	// change params - longer window with smaller exclude size
	rl.windowSize = 4
	rl.excludeSize = 2
	startIndex = currentRound
	for i := 0; i < 2*len(peerIDs); i++ {
		currentRound++
		require.NoError(t, processRound(rl, currentRound))
	}
	for i := int(startIndex); i <= int(currentRound); i++ {
		checkAuthor(rl, uint64(i))
	}

	// change params - the case AB is currently limited to, WS=ExS=1
	rl.windowSize = 1
	rl.excludeSize = 1
	startIndex = currentRound
	for i := 0; i < 2*len(peerIDs); i++ {
		currentRound++
		require.NoError(t, processRound(rl, currentRound))
	}
	for i := int(startIndex); i <= int(currentRound); i++ {
		checkAuthor(rl, uint64(i))
	}
}
