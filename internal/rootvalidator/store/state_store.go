package store

import (
	gocrypto "crypto"
	"sync"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/util"
)

const (
	ErrStateIsNil                  = "err state is nil"
	ErrInvalidRound                = "err invalid round number"
	ErrInvalidRootHash             = "invalid root hash"
	ErrInvalidCertificates         = "missing unicity certificates"
	ErrIllegalNewRound             = "illegal new round number in new state"
	ErrPersistentStoreBackendIsNil = "persistent store backend is nil"
)

type RootState struct {
	LatestRound    uint64
	LatestRootHash []byte
	Certificates   map[protocol.SystemIdentifier]*certificates.UnicityCertificate
}

type InMemState struct {
	state RootState
	mu    sync.Mutex
}

type PersistentRootState struct {
	cachedState  RootState
	storeBackend *BoltStore
	mu           sync.Mutex
}

func (r *RootState) Update(newState RootState) {
	r.LatestRound = newState.LatestRound
	r.LatestRootHash = newState.LatestRootHash
	// Update changed UC's
	for id, uc := range newState.Certificates {
		r.Certificates[id] = uc
	}
}

func (r RootState) GetStateHash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	hasher.Write(util.Uint64ToBytes(r.LatestRound))
	for _, uc := range r.Certificates {
		uc.AddToHasher(hasher)
	}
	return hasher.Sum(nil)
}

func (r *RootState) IsValid() error {
	if r == nil {
		return errors.New(ErrStateIsNil)
	}
	if r.LatestRound < 1 {
		return errors.New(ErrInvalidRound)
	}
	if len(r.LatestRootHash) < gocrypto.SHA256.Size() {
		return errors.New(ErrInvalidRootHash)
	}
	if len(r.Certificates) == 0 {
		return errors.New(ErrInvalidCertificates)
	}
	return nil
}

// NewInMemStateStore stores state in volatile memory only, everything is lost on exit
func NewInMemStateStore(hashAlgorithm gocrypto.Hash) *InMemState {
	return &InMemState{
		state: RootState{
			LatestRound:    0,
			LatestRootHash: make([]byte, hashAlgorithm.Size()),
			Certificates:   make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate),
		},
	}
}

func (s *InMemState) Save(newState RootState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := checkRoundNumber(s.state, newState); err != nil {
		return err
	}
	s.state.Update(newState)
	return nil
}

func (s *InMemState) Get() (RootState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, nil
}

// NewPersistentStateStore persists state using the persistent storage provider interface
// Currently uses RootSate as cache (probably should be refactored)
func NewPersistentStateStore(store *BoltStore) (*PersistentRootState, error) {
	if store == nil {
		return nil, errors.New(ErrPersistentStoreBackendIsNil)
	}
	// Read last state from persistent store
	latestRound, err := store.ReadLatestRoundNumber()
	if err != nil {
		return nil, err
	}
	latestRootHash, err := store.ReadLatestRoundRootHash()
	if err != nil {
		return nil, err
	}
	certs, err := store.ReadAllUC()
	if err != nil {
		return nil, err
	}
	return &PersistentRootState{
		cachedState:  RootState{LatestRound: latestRound, Certificates: certs, LatestRootHash: latestRootHash},
		storeBackend: store,
	}, nil
}

func (p *PersistentRootState) Save(newState RootState) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	// round number sanity check
	if err := checkRoundNumber(p.cachedState, newState); err != nil {
		return err
	}
	// persist state
	if err := p.storeBackend.WriteState(newState); err != nil {
		return err
	}
	// update local cache
	p.cachedState.Update(newState)
	return nil
}

func (p *PersistentRootState) Get() (RootState, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cachedState, nil
}

// checkRoundNumber makes sure that the round number monotonically increases
// This will become obsolete in distributed root chain solution, then it just has to be bigger and caps are possible
func checkRoundNumber(current, newState RootState) error {
	// Round number must be increasing
	if current.LatestRound+1 != newState.LatestRound {
		return errors.New(ErrIllegalNewRound)
	}
	return nil
}
