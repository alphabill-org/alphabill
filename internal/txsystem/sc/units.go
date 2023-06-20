package sc

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/state"
)

type Program struct{}

func (p *Program) Write(hasher hash.Hash) {
}

func (p *Program) SummaryValueInput() uint64 {
	return 0
}

func (p *Program) Copy() state.UnitData {
	return &Program{}
}

type StateVariable struct {
	value []byte
}

func (s *StateVariable) AddToHasher(hasher hash.Hash) {
	hasher.Write(s.value)
}

func (s *StateVariable) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}
