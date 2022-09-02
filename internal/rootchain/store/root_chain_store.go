package store

import (
	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
)

type RootChainStore interface {
	AddUC(p.SystemIdentifier, *certificates.UnicityCertificate)
	GetUC(p.SystemIdentifier) *certificates.UnicityCertificate
	UCCount() int
	AddIR(p.SystemIdentifier, *certificates.InputRecord)
	GetIR(p.SystemIdentifier) *certificates.InputRecord
	GetAllIRs() map[p.SystemIdentifier]*certificates.InputRecord
	GetRoundNumber() uint64
	GetPreviousRoundRootHash() []byte
	PrepareNextRound([]byte) uint64
}
