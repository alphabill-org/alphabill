package store

import "github.com/alphabill-org/alphabill/internal/certificates"

type RootChainStore interface {
	AddUC(systemIdentifier string, certificate *certificates.UnicityCertificate)
	GetUC(systemIdentifier string) *certificates.UnicityCertificate
	UCCount() int
	GetRoundNumber() uint64
	GetPreviousRoundRootHash() []byte
	PrepareNextRound([]byte)
}
