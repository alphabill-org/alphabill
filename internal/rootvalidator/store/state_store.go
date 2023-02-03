package store

import (
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

const (
	stateKey = "state"
)

type (
	RootState struct {
		Round        uint64                                                         `json:"latestRound"`
		RootHash     []byte                                                         `json:"latestRootHash"`
		Certificates map[protocol.SystemIdentifier]*certificates.UnicityCertificate `json:"certificates"`
	}

	PersistentStore interface {
		Read(k string, v any) error
		Write(k string, v any) error
	}

	Conf struct {
		db PersistentStore
	}

	StateStore struct {
		state *RootState
		db    PersistentStore
		mu    sync.Mutex
	}

	Option func(c *Conf)
)

func WithDBStore(p PersistentStore) Option {
	return func(c *Conf) {
		c.db = p
	}
}

func NewRootState() *RootState {
	return &RootState{
		Round:        0,
		RootHash:     nil,
		Certificates: map[protocol.SystemIdentifier]*certificates.UnicityCertificate{},
	}
}

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

func loadConf(opts []Option) *Conf {
	conf := &Conf{
		db: nil,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}

func New(opts ...Option) *StateStore {
	config := loadConf(opts)
	return &StateStore{
		db:    config.db,
		state: nil,
	}
}

func NewFromGenesis(genesis *genesis.RootGenesis, opts ...Option) (*StateStore, error) {
	if genesis == nil {
		return nil, fmt.Errorf("genesis is nil")
	}
	config := loadConf(opts)
	if config.db == nil {
		return &StateStore{
			db:    nil,
			state: NewRootStateFromGenesis(genesis),
		}, nil
	}
	lastState := NewRootState()
	var err error = nil
	if err = config.db.Read(stateKey, lastState); err != nil && err != ErrValueEmpty {
		return nil, err
	}
	// DB is empty, initiate store
	if err == ErrValueEmpty {
		// initiate DB
		lastState = NewRootStateFromGenesis(genesis)
		if err = config.db.Write(stateKey, lastState); err != nil {
			return nil, fmt.Errorf("init DB error, %w", err)
		}
	}
	return &StateStore{
		db:    config.db,
		state: lastState,
	}, nil
}

func (s *StateStore) IsEmpty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil && s.db == nil {
		return true
	}
	if s.db != nil {
		lastState := &RootState{}
		if err := s.db.Read(stateKey, lastState); err == ErrValueEmpty {
			return true
		}
	}
	return false
}

func (s *StateStore) Save(newState *RootState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if newState == nil {
		return fmt.Errorf("state is nil")
	}
	if s.state == nil {
		s.state = newState
	} else {
		// round number sanity check
		if err := checkRoundNumber(s.state, newState); err != nil {
			return err
		}
		// update local cache
		s.state.Update(newState)
	}
	// persist state
	if s.db != nil {
		if err := s.db.Write(stateKey, newState); err != nil {
			return err
		}
	}
	return nil
}

func (s *StateStore) Get() (*RootState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		if s.db == nil {
			return nil, ErrValueEmpty
		}
		lastState := NewRootState()
		// also returns ErrValueEmpty if no state is stored
		if err := s.db.Read(stateKey, lastState); err != nil {
			return nil, err
		}
		s.state = lastState
	}
	return s.state, nil
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
