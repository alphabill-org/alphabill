package store

import (
	"github.com/alphabill-org/alphabill/internal/certificates"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/pkg/errors"
)

// InMemoryRootChainStore keeps track of latest unicity certificates.
type InMemoryRootChainStore struct {
	ucStore               map[p.SystemIdentifier]*certificates.UnicityCertificate
	inputRecords          map[p.SystemIdentifier]*certificates.InputRecord // input records ready for certification. key is system identifier
	roundNumber           uint64                                           // current round number
	previousRoundRootHash []byte                                           // previous round root hash
}

// NewInMemoryRootChainStore returns a new empty InMemoryRootChainStore.
func NewInMemoryRootChainStore() *InMemoryRootChainStore {
	s := InMemoryRootChainStore{
		ucStore:               make(map[p.SystemIdentifier]*certificates.UnicityCertificate),
		inputRecords:          make(map[p.SystemIdentifier]*certificates.InputRecord),
		roundNumber:           0,
		previousRoundRootHash: nil,
	}
	return &s
}

func (u *InMemoryRootChainStore) Init(prevStateHash []byte, ucs []*certificates.UnicityCertificate, round uint64) error {
	defer logger.Info("Store initialised")

	if prevStateHash == nil {
		return errors.New("previous hash is nil")
	}
	if round < 1 {
		return errors.New("invalid round number 0")
	}
	u.roundNumber = round
	u.previousRoundRootHash = prevStateHash
	for _, cert := range ucs {
		u.ucStore[p.SystemIdentifier(cert.UnicityTreeCertificate.SystemIdentifier)] = cert
	}
	u.inputRecords = make(map[p.SystemIdentifier]*certificates.InputRecord)
	return nil
}

func (u *InMemoryRootChainStore) GetInitiated() bool {
	if u.roundNumber < 1 {
		return false
	}
	return true
}

// GetUC returns the unicity certificate or nil if not found.
func (u *InMemoryRootChainStore) GetUC(id p.SystemIdentifier) *certificates.UnicityCertificate {
	return u.ucStore[id]
}

// UCCount returns the total number of unicity certificates
func (u *InMemoryRootChainStore) UCCount() int {
	return len(u.ucStore)
}

func (u *InMemoryRootChainStore) AddIR(id p.SystemIdentifier, ir *certificates.InputRecord) {
	u.inputRecords[id] = ir
}

func (u *InMemoryRootChainStore) GetIR(id p.SystemIdentifier) *certificates.InputRecord {
	return u.inputRecords[id]
}

func (u *InMemoryRootChainStore) GetAllIRs() map[p.SystemIdentifier]*certificates.InputRecord {
	target := make(map[p.SystemIdentifier]*certificates.InputRecord, len(u.inputRecords))
	for k, v := range u.inputRecords {
		target[k] = v
	}
	return target
}

func (u *InMemoryRootChainStore) GetRoundNumber() uint64 {
	return u.roundNumber
}

func (u *InMemoryRootChainStore) GetPreviousRoundRootHash() []byte {
	return u.previousRoundRootHash
}

func (u *InMemoryRootChainStore) SaveState(previousRoundRootHash []byte, ucs []*certificates.UnicityCertificate, newRoundNumber uint64) {
	if u.roundNumber+1 != newRoundNumber {
		panic(errors.Errorf("Inconsistent round number, current=%v, new=%v", u.roundNumber, newRoundNumber))
	}
	u.roundNumber++
	u.previousRoundRootHash = previousRoundRootHash
	for _, cert := range ucs {
		u.ucStore[p.SystemIdentifier(cert.UnicityTreeCertificate.SystemIdentifier)] = cert
	}
	u.inputRecords = make(map[p.SystemIdentifier]*certificates.InputRecord)
}
