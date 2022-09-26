package rootchain

import (
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
)

const (
	ErrIllegalNewRound = "illegal new round number in new state"
)

type RootStatePersistReadWriter interface {
	ReadLatestRoundNumber() uint64
	ReadLatestRoundRootHash() []byte
	// ReadAllUC reads all persisted Unicity certificates, returns nil if none are stored
	ReadAllUC() map[protocol.SystemIdentifier]*certificates.UnicityCertificate
	// WriteState atomically update persistent store, when this returns store must be complete
	WriteState(prevStateHash []byte, ucs []*certificates.UnicityCertificate, newRoundNumber uint64)
}

type RootState struct {
	latestRound    uint64
	latestRootHash []byte
	ucs            map[protocol.SystemIdentifier]*certificates.UnicityCertificate
}

// NewVolatileStateStore stores state in volatile memory only, everything is lost on exit
func NewVolatileStateStore(hashAlgorithm gocrypto.Hash) RootStateStore {
	return &RootState{
		latestRound:    0,
		latestRootHash: make([]byte, hashAlgorithm.Size()),
		ucs:            make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate),
	}
}

func (s *RootState) SaveState(prevHash []byte, ucs []*certificates.UnicityCertificate, newRoundNumber uint64) {
	// safety check, state round number must advance linearly
	if s.latestRound+1 != newRoundNumber {
		panic(errors.New(ErrIllegalNewRound))
	}
	s.latestRound = newRoundNumber
	s.latestRootHash = prevHash
	// Update changed UC's
	for _, uc := range ucs {
		id := protocol.SystemIdentifier(uc.UnicityTreeCertificate.SystemIdentifier)
		s.ucs[id] = uc
	}
}

func (s *RootState) GetLatestRoundNumber() uint64 {
	return s.latestRound
}

func (s *RootState) GetLatestRoundRootHash() []byte {
	return s.latestRootHash
}

func (s *RootState) GetUC(id protocol.SystemIdentifier) *certificates.UnicityCertificate {
	uc, found := s.ucs[id]
	if !found {
		return nil
	}
	return uc
}

func (s *RootState) UCCount() int {
	return len(s.ucs)
}

type PersistentRootState struct {
	cachedState  RootState
	storeBackend RootStatePersistReadWriter
}

// NewPersistentStateStoreFromDb persists state using the persistent storage provider interface
// Currently uses RootSate as cache (probably should be refactored)
func NewPersistentStateStoreFromDb(store RootStatePersistReadWriter) RootStateStore {
	// Init to default
	// Get the latest state from DB
	return &PersistentRootState{
		cachedState:  RootState{latestRound: store.ReadLatestRoundNumber(), latestRootHash: store.ReadLatestRoundRootHash(), ucs: store.ReadAllUC()},
		storeBackend: store,
	}
}

func (p PersistentRootState) SaveState(prevHash []byte, ucs []*certificates.UnicityCertificate, newRoundNumber uint64) {
	// update cache
	p.cachedState.SaveState(prevHash, ucs, newRoundNumber)
	// persist state
	p.storeBackend.WriteState(prevHash, ucs, newRoundNumber)
}

func (p PersistentRootState) GetLatestRoundNumber() uint64 {
	return p.cachedState.GetLatestRoundNumber()
}

func (p PersistentRootState) GetLatestRoundRootHash() []byte {
	return p.cachedState.latestRootHash
}

func (p PersistentRootState) GetUC(id protocol.SystemIdentifier) *certificates.UnicityCertificate {
	return p.cachedState.GetUC(id)
}

func (p PersistentRootState) UCCount() int {
	return p.cachedState.UCCount()
}
