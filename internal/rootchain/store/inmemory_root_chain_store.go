package store

import (
	"github.com/alphabill-org/alphabill/internal/certificates"
)

// UnicityCertificatesStore keeps track of latest unicity certificates.
type UnicityCertificatesStore struct {
	ucStore     map[string]*certificates.UnicityCertificate
	roundNumber uint64 // current round number
}

// NewUnicityCertificateStore returns a new empty UnicityCertificatesStore.
func NewUnicityCertificateStore() *UnicityCertificatesStore {
	s := UnicityCertificatesStore{
		ucStore:     make(map[string]*certificates.UnicityCertificate),
		roundNumber: 1,
	}
	return &s
}

// AddUC adds or replaces the unicity certificate with given identifier.
func (u *UnicityCertificatesStore) AddUC(identifier string, certificate *certificates.UnicityCertificate) {
	u.ucStore[identifier] = certificate
}

// GetUC returns the unicity certificate or nil if not found.
func (u *UnicityCertificatesStore) GetUC(id string) *certificates.UnicityCertificate {
	return u.ucStore[id]
}

// UCCount returns the total number of unicity certificates
func (u *UnicityCertificatesStore) UCCount() int {
	return len(u.ucStore)
}

func (u *UnicityCertificatesStore) GetRoundNumber() uint64 {
	return u.roundNumber
}

func (u *UnicityCertificatesStore) IncrementRoundNumber() uint64 {
	u.roundNumber++
	return u.roundNumber
}
