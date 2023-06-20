package sc

import (
	"hash"

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
