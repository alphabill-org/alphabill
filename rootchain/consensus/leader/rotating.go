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
	if contRounds < 1 || contRounds > uint32(len(validators)) {
		return nil, fmt.Errorf("invalid number of continuous rounds %d (must be between 1 and %d)", contRounds, len(validators))
	}
	return &RoundRobin{validators: validators, nofRounds: contRounds}, nil
}

func (r *RoundRobin) GetLeaderForRound(round uint64) peer.ID {
	index := uint32(round/uint64(r.nofRounds)) % uint32(len(r.validators))
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
