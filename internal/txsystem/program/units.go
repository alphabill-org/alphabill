package program

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
)

type Program struct {
	wasm     []byte
	initData []byte
}

type StateFile struct {
	bytes []byte
}

func (p *Program) AddToHasher(hash.Hash) {
	// Nothing to add
}

func (p *Program) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}

func (p *Program) Wasm() []byte {
	return p.wasm
}

func (p *Program) InitData() []byte {
	return p.initData
}

func (s *StateFile) AddToHasher(hasher hash.Hash) {
	hasher.Write(s.bytes)
}

func (s *StateFile) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}

func (s *StateFile) GetBytes() []byte {
	return s.bytes
}
