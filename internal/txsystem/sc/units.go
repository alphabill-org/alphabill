package sc

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
)

type Program struct{}

type StateVariable struct {
	value []byte
}

func (p *Program) AddToHasher(hash.Hash) {
	// Nothing to add
}

func (p *Program) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}

func (s *StateVariable) AddToHasher(hasher hash.Hash) {
	hasher.Write(s.value)
}

func (s *StateVariable) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}
