package store

import (
	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
)

type RootChainStore interface {
	// Init commits initial (genesis) state to store.
	Init(pervHash []byte, uc []*certificates.UnicityCertificate, round uint64) error
	// GetInitiated returns true if InitStore has been successfully executed
	GetInitiated() bool
	// GetUC returns matching Unicity Certificate. Nil is returned if not found or store not initiated.
	GetUC(p.SystemIdentifier) *certificates.UnicityCertificate
	// UCCount returns number of Unicity Certificates stored.
	UCCount() int
	// AddIR adds IR record to be certified
	AddIR(p.SystemIdentifier, *certificates.InputRecord)
	// GetIR returns matching IR record or nil if not found
	GetIR(p.SystemIdentifier) *certificates.InputRecord
	// GetAllIRs returns all stored IR records. Empty map if none are stored.
	GetAllIRs() map[p.SystemIdentifier]*certificates.InputRecord
	// GetRoundNumber returns current round number. 0 if store is not initiated.
	GetRoundNumber() uint64
	// GetPreviousRoundRootHash returns previous round root hash or nil if store is not initiated.
	GetPreviousRoundRootHash() []byte
	// SaveState commits new state to store.
	SaveState(pervHash []byte, uc []*certificates.UnicityCertificate, round uint64)
}
