package store

import (
	gocrypto "crypto"
	"github.com/alphabill-org/alphabill/internal/certificates"
)

// InMemoryRootChainStore keeps track of latest unicity certificates.
type InMemoryRootChainStore struct {
	ucStore               map[string]*certificates.UnicityCertificate
	roundNumber           uint64 // current round number
	previousRoundRootHash []byte // previous round root hash
}

// NewInMemoryRootChainStore returns a new empty InMemoryRootChainStore.
func NewInMemoryRootChainStore() *InMemoryRootChainStore {
	s := InMemoryRootChainStore{
		ucStore:               make(map[string]*certificates.UnicityCertificate),
		roundNumber:           1,
		previousRoundRootHash: make([]byte, gocrypto.SHA256.Size()),
	}
	return &s
}

// AddUC adds or replaces the unicity certificate with given identifier.
func (u *InMemoryRootChainStore) AddUC(identifier string, certificate *certificates.UnicityCertificate) {
	u.ucStore[identifier] = certificate
}

// GetUC returns the unicity certificate or nil if not found.
func (u *InMemoryRootChainStore) GetUC(id string) *certificates.UnicityCertificate {
	return u.ucStore[id]
}

// UCCount returns the total number of unicity certificates
func (u *InMemoryRootChainStore) UCCount() int {
	return len(u.ucStore)
}

func (u *InMemoryRootChainStore) GetRoundNumber() uint64 {
	return u.roundNumber
}

func (u *InMemoryRootChainStore) GetPreviousRoundRootHash() []byte {
	return u.previousRoundRootHash
}

func (u *InMemoryRootChainStore) PrepareNextRound(previousRoundRootHash []byte) {
	u.roundNumber++
	u.previousRoundRootHash = previousRoundRootHash
}
