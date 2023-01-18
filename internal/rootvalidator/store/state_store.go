package store

import (
	"fmt"
	"sync"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
)

const (
	ErrIllegalNewRound = "illegal new round number in new state"
)

type (
	RootState struct {
		LatestRound    uint64
		LatestRootHash []byte
		Certificates   map[protocol.SystemIdentifier]*certificates.UnicityCertificate
	}

	PersistentStore interface {
		Read() (*RootState, error)
		Write(newState *RootState) error
	}

	Conf struct {
		db PersistentStore
	}

	StateStore struct {
		state *RootState
		conf  *Conf
		mu    sync.Mutex
	}

	Option func(c *Conf)
)

func WithDBStore(store PersistentStore) Option {
	return func(c *Conf) {
		c.db = store
	}
}

func NewRootState() *RootState {
	return &RootState{
		LatestRound:    0,
		LatestRootHash: nil,
		Certificates:   map[protocol.SystemIdentifier]*certificates.UnicityCertificate{},
	}
}

func (r *RootState) Update(newState *RootState) {
	r.LatestRound = newState.LatestRound
	r.LatestRootHash = newState.LatestRootHash
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

func New(opts ...Option) (*StateStore, error) {
	config := loadConf(opts)
	if config.db == nil {
		return &StateStore{
			conf:  config,
			state: NewRootState(),
		}, nil
	}
	lastState, err := config.db.Read()
	if err != nil {
		return nil, err
	}
	return &StateStore{
		state: lastState,
		conf:  config,
	}, nil
}

// NewInMemStateStore stores state in volatile memory only, everything is lost on exit
func NewInMemStateStore() *StateStore {
	return &StateStore{
		conf:  &Conf{db: nil},
		state: NewRootState(),
	}
}

func (s *StateStore) Save(newState *RootState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if newState == nil {
		return fmt.Errorf("state is nil")
	}
	// round number sanity check
	if err := checkRoundNumber(s.state, newState); err != nil {
		return err
	}
	// update local cache
	s.state.Update(newState)
	// persist state
	if s.conf.db != nil {
		if err := s.conf.db.Write(newState); err != nil {
			return err
		}
	}
	return nil
}

func (s *StateStore) Get() (*RootState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}

// checkRoundNumber makes sure that the round number monotonically increases
// This will become obsolete in distributed root chain solution, then it just has to be bigger and caps are possible
func checkRoundNumber(current, newState *RootState) error {
	// Round number must be increasing
	if current.LatestRound >= newState.LatestRound {
		return errors.New(ErrIllegalNewRound)
	}
	return nil
}
