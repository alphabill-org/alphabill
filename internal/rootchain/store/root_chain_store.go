package store

import (
	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
)

type RootChainStore interface {
	GetUC(p.SystemIdentifier) *certificates.UnicityCertificate
	UCCount() int
	AddIR(p.SystemIdentifier, *certificates.InputRecord)
	GetIR(p.SystemIdentifier) *certificates.InputRecord
	GetAllIRs() map[p.SystemIdentifier]*certificates.InputRecord
	GetRoundNumber() uint64
	GetPreviousRoundRootHash() []byte
	SaveState([]byte, []*certificates.UnicityCertificate, uint64)
}
