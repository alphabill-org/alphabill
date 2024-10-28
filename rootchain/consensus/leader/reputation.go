package leader

import (
	"fmt"
	"sort"
	"sync"

	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

/*
NewReputationBased creates "leader election based on reputation" strategy implementation.

  - "validators" is (sorted!) list of peer IDs to use for round-robin leader selection
    when there is no elected leader for the round;
  - the "windowSize" is number of committed blocks to use to determine list of active
    validators (ie those which voted). It must not be larger than the "block history"
    available to the blockLoader, IOW the "blockLoader" must be able to return blocks
    at least inside the window.
  - "excludeSize" is the number of most recent block authors to exclude form candidate
    list when electing leader for the next round. Generally should be between f and 2f
    (where f is max allowed number of faulty nodes).
*/
func NewReputationBased(validators []peer.ID, windowSize, excludeSize int) (*ReputationBased, error) {
	if len(validators) == 0 {
		return nil, fmt.Errorf("peer list (validators) must not be empty")
	}
	if len(validators) <= excludeSize {
		return nil, fmt.Errorf("excludeSize value must be smaller than the number of validators in the system (%d validators, exclude %d)", len(validators), excludeSize)
	}

	if windowSize == 0 {
		return nil, fmt.Errorf("window size must be greater than zero")
	}

	return &ReputationBased{
		windowSize:  windowSize,
		excludeSize: excludeSize,
		validators:  validators,
	}, nil
}

type roundLeader struct {
	round  uint64
	leader peer.ID
}

/*
ReputationBased implements reputation based leader election strategy.
Zero value is not usable, use constructor!

Remarks:
  - same leader might be elected for up to three consecutive rounds - leader of
    the round R is not eligible to be leader for rounds [R+3 .. R+3+excludeSize-1];
*/
type ReputationBased struct {
	windowSize  int // number of latest commits to take into account when determining which validators are active
	excludeSize int // number of excluded authors of last committed blocks (should be between f and 2f)

	validators peer.IDSlice // sorted! list of all peers in the system

	// Elected leaders.
	// We do not (need to) keep history and we can only elect leader for the next
	// round so we need two slots - current round and next round.
	// One additional slot as a buffer for cases where election happens while some
	// other part of the manager still needs to know (now committed) round leader?
	leaders [3]roundLeader
	curIdx  int        // index of the latest election round data in the "leaders" slice
	m       sync.Mutex // this lock must be held while operating on "curIdx" and/or "leaders"
}

/*
GetLeaderForRound returns either elected leader or (in case the round doesn't have
elected leader) falls back to round-robin of all validators.
Undefined behavior for round==0.
*/
func (rb *ReputationBased) GetLeaderForRound(round uint64) peer.ID {
	rb.m.Lock()
	defer rb.m.Unlock()

	for _, l := range rb.leaders {
		if l.round == round {
			return l.leader
		}
	}
	return pickLeader(rb.validators, round)
}

/*
GetNodes returns all currently active root nodes
*/
func (rb *ReputationBased) GetNodes() []peer.ID {
	return rb.validators
}

/*
Update triggers leader election for the next round.
Returns error when election fails or QC and currentRound combination does not trigger election.
"currentRound" - what PaceMaker considers to be the current round at the time QC is processed.
"blockLoader" - a callback into block store which allows to load block data of given
round, it is expected to return either valid block or error.
*/
func (rb *ReputationBased) Update(qc *abtypes.QuorumCert, currentRound uint64, blockLoader BlockLoader) error {
	exR := qc.GetParentRound()
	qcR := qc.GetRound()
	if exR+1 != qcR || qcR+1 != currentRound {
		return fmt.Errorf("not updating leaders because rounds are not consecutive {parent: %d, QC: %d, current: %d}", exR, qcR, currentRound)
	}

	leader, err := rb.electLeader(qc, blockLoader)
	if err != nil {
		return fmt.Errorf("failed to elect leader for round %d: %w", currentRound+1, err)
	}

	rb.m.Lock()
	idx := rb.slotIndex(currentRound + 1)
	rb.leaders[idx].round = currentRound + 1
	rb.leaders[idx].leader = leader
	rb.m.Unlock()

	return nil
}

/*
slotIndex returns "leaders" index into which election result for the "round" should be stored.
Same index is returned when calling with the same "round" value multiple times in a row.
*/
func (rb *ReputationBased) slotIndex(round uint64) int {
	if round == rb.leaders[rb.curIdx].round {
		return rb.curIdx
	}

	if rb.curIdx++; rb.curIdx == len(rb.leaders) {
		rb.curIdx = 0
	}
	return rb.curIdx
}

func (rb *ReputationBased) electLeader(qc *abtypes.QuorumCert, blockLoader BlockLoader) (peer.ID, error) {
	qcRound := qc.GetRound()
	extra := qc.LedgerCommitInfo.PreviousHash
	leaderSeed := qcRound + (uint64(extra[0]) | uint64(extra[1])<<8 | uint64(extra[2])<<16 | uint64(extra[3])<<24)

	authors := make(map[string]struct{}) // block authors of the recent rounds
	active := make(map[string]struct{})  // validators that signed the committed blocks
	round := qc.GetParentRound()
	for i := 0; i < rb.windowSize || len(authors) < rb.excludeSize; i++ {
		block, err := blockLoader(round)
		if err != nil {
			return UnknownLeader, fmt.Errorf("failed to load round %d block data (starting QC round %d, iteration %d): %w", round, qcRound, i, err)
		}

		if len(authors) < rb.excludeSize {
			authors[block.BlockData.Author] = struct{}{}
		}
		if i < rb.windowSize {
			for signerID := range qc.Signatures {
				active[signerID] = struct{}{}
			}
		}

		qc = block.BlockData.Qc
		round = qc.GetRound()
	}

	for id := range authors {
		delete(active, id)
	}
	if len(active) == 0 {
		return UnknownLeader, fmt.Errorf("no active validators left after eliminating %d recent authors", len(authors))
	}

	leader := pickLeader(toSortedSlice(active), leaderSeed)
	id, err := peer.Decode(leader)
	if err != nil {
		return UnknownLeader, fmt.Errorf("invalid peer id %q: %w", leader, err)
	}
	return id, nil
}

/*
pickLeader selects an item from "leaders" slice on a round-robin basis
using (current)round as a "seed". The "leaders" slice must not be empty.
*/
func pickLeader[T any](leaders []T, round uint64) T {
	return leaders[round%uint64(len(leaders))]
}

/*
toSortedSlice returns keys of the "leaders" map as a sorted slice of strings.
*/
func toSortedSlice(leaders map[string]struct{}) []string {
	s := make([]string, len(leaders))
	i := 0
	for id := range leaders {
		s[i] = id
		i++
	}
	sort.Strings(s)
	return s
}
