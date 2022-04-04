package rootchain

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
)

// unicityCertificatesStore keeps track of latest unicity certificates.
type unicityCertificatesStore map[string]*certificates.UnicityCertificate

// newUnicityCertificateStore returns a new empty unicityCertificatesStore.
func newUnicityCertificateStore() *unicityCertificatesStore {
	s := unicityCertificatesStore(make(map[string]*certificates.UnicityCertificate))
	return &s
}

// put adds or replaces the unicity certificate with given identifier.
func (u *unicityCertificatesStore) put(identifier string, certificate *certificates.UnicityCertificate) {
	(*u)[identifier] = certificate
}

// get returns the unicity certificate or nil if not found.
func (u *unicityCertificatesStore) get(id string) *certificates.UnicityCertificate {
	return (*u)[id]
}

// size returns the total number of unicity certificates
func (u *unicityCertificatesStore) size() int {
	return len(*u)
}
