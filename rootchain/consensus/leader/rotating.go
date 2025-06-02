package leader

import (
	"errors"
	"fmt"

	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/libp2p/go-libp2p/core/peer"
)

const UnknownLeader = ""

type RoundRobin struct {
	validators []peer.ID
	nofRounds  uint32
}

// NewRoundRobin returns round-robin leader selection algorithm based on node identifiers.
// It is assumed that the order of node identifiers is the same (e.g. alphabetical) for all validators.
// "contRounds" - for how many continuous rounds each peer is considered to be the leader.
func NewRoundRobin(validators []peer.ID, contRounds uint32) (*RoundRobin, error) {
	if len(validators) < 1 {
		return nil, errors.New("empty root validator node id list")
	}
	nofValidators := uint32(len(validators)) /* #nosec G115 its unlikely that validators count exceeds uint32 max value */
	if contRounds < 1 || contRounds > nofValidators {
		return nil, fmt.Errorf("invalid number of continuous rounds %d (must be between 1 and %d)", contRounds, nofValidators)
	}
	return &RoundRobin{validators: validators, nofRounds: contRounds}, nil
}

func (r *RoundRobin) GetLeaderForRound(round uint64) peer.ID {
	index := (round / uint64(r.nofRounds)) % uint64(len(r.validators))
	return r.validators[index]
}

/*
GetNodes returns all currently active root nodes
*/
func (r *RoundRobin) GetNodes() []peer.ID {
	return r.validators
}

func (r *RoundRobin) Update(qc *abtypes.QuorumCert, currentRound uint64, b BlockLoader) error {
	return nil
}
