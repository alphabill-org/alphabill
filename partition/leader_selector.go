package partition

import (
	"crypto"
	"sync"

	"github.com/alphabill-org/alphabill/types"
	"github.com/holiman/uint256"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	UnknownLeader = ""
)

type (
	LeaderSelector interface {
		UpdateLeader(uc *types.UnicityCertificate, validators []peer.ID)
		LeaderFunc(uc *types.UnicityCertificate, validators []peer.ID) peer.ID
		IsLeader(peer peer.ID) bool
		GetLeader() peer.ID
	}

	// DefaultLeaderSelector is used to select a leader from the validator pool.
	DefaultLeaderSelector struct {
		mutex  sync.Mutex
		leader peer.ID
	}
)

func NewDefaultLeaderSelector() *DefaultLeaderSelector {
	return &DefaultLeaderSelector{leader: UnknownLeader}
}

// IsLeader returns true it the given peer is the leader and must propose the next block.
func (l *DefaultLeaderSelector) IsLeader(peerID peer.ID) bool {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader == peerID
}

func (l *DefaultLeaderSelector) GetLeader() peer.ID {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return l.leader
}

// UpdateLeader updates the leader (next block proposer). If input is nil then leader is set to UnknownLeader.
func (l *DefaultLeaderSelector) UpdateLeader(uc *types.UnicityCertificate, validators []peer.ID) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.leader = l.LeaderFunc(uc, validators)
}

// LeaderFunc calculates new leader from Unicity Certificate.
// For details see Yellowpaper Algorithm 1 "State and Initialization" - "function leaderfunc(uc)" description.
func (l *DefaultLeaderSelector) LeaderFunc(uc *types.UnicityCertificate, validators []peer.ID) peer.ID {
	// We don't need the lock because we don't change the state of the struct.
	if uc == nil {
		return UnknownLeader
	}
	peerCount := uint64(len(validators))
	hasher := crypto.SHA256.New()
	uc.AddToHasher(hasher)
	hash := hasher.Sum(nil)
	x := uint256.NewInt(0).SetBytes(hash)
	i := uint256.NewInt(0)
	i.Mod(x, uint256.NewInt(peerCount))
	index := i.Uint64()
	if index > peerCount {
		return UnknownLeader
	}
	return validators[index]
}
