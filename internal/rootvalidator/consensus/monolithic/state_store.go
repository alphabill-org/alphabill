package monolithic

import (
	"fmt"
	"path"
	"sync"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/rootdb"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/rootdb/boltdb"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/rootdb/memorydb"
)

const (
	BoltRootChainStoreFileName = "rootchain.db"
	stateKey                   = "state"
)

type (
	RootState struct {
		Round        uint64                                                         `json:"latestRound"`
		RootHash     []byte                                                         `json:"latestRootHash"`
		Certificates map[protocol.SystemIdentifier]*certificates.UnicityCertificate `json:"certificates"`
	}

	StateStore struct {
		state *RootState
		db    rootdb.KeyValueDB
		mu    sync.Mutex
	}
)

func NewRootStateFromGenesis(rg *genesis.RootGenesis) *RootState {
	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range rg.Partitions {
		identifier := partition.GetSystemIdentifierString()
		certs[identifier] = partition.Certificate
	}
	// If not initiated, save genesis file to store
	return &RootState{Round: rg.GetRoundNumber(), Certificates: certs, RootHash: rg.GetRoundHash()}
}

func (r *RootState) Update(newState *RootState) {
	r.Round = newState.Round
	r.RootHash = newState.RootHash
	// Update changed UC's
	for id, uc := range newState.Certificates {
		r.Certificates[id] = uc
	}
}

func newWithMemoryDB(rg *genesis.RootGenesis) (*StateStore, error) {
	db := memorydb.New()
	state := NewRootStateFromGenesis(rg)
	if err := db.Write([]byte(stateKey), state); err != nil {
		return nil, err
	}
	return &StateStore{
		db:    db,
		state: state,
	}, nil
}

func newWithBoltDB(rg *genesis.RootGenesis, dbPath string) (*StateStore, error) {
	var state *RootState
	db, err := boltdb.New(path.Join(dbPath, BoltRootChainStoreFileName))
	if err != nil {
		return nil, fmt.Errorf("bolt db init failed, %w", err)
	}
	if db.Empty() {
		state = NewRootStateFromGenesis(rg)
		if err = db.Write([]byte(stateKey), state); err != nil {
			return nil, fmt.Errorf("bolt db genesis write failed, %w", err)
		}
	} else {
		// read last stored state
		var lastState RootState
		found, err := db.Read([]byte(stateKey), &lastState)
		if found == false || err != nil {
			return nil, fmt.Errorf("read last state from bolt db failed, %w", err)
		}
		state = &lastState
	}
	return &StateStore{
		db:    db,
		state: state,
	}, nil
}

func NewStateStore(rg *genesis.RootGenesis, dbPath string) (*StateStore, error) {
	if len(dbPath) == 0 {
		return newWithMemoryDB(rg)
	} else {
		return newWithBoltDB(rg, dbPath)
	}
}

func (s *StateStore) Save(newState *RootState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if newState == nil {
		return fmt.Errorf("state is nil")
	}
	if err := checkRoundNumber(s.state, newState); err != nil {
		return err
	}
	// update local cache
	s.state.Update(newState)
	// persist state
	return s.db.Write([]byte(stateKey), newState)
}

func (s *StateStore) Get() *RootState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// checkRoundNumber makes sure that the round number monotonically increases
// This will become obsolete in distributed root chain solution, then it just has to be bigger and caps are possible
func checkRoundNumber(current, newState *RootState) error {
	// Round number must be increasing
	if current.Round >= newState.Round {
		return fmt.Errorf("error new round %v is in past, latest stored round %v", newState.Round, current.Round)
	}
	return nil
}
