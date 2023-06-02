package program

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/rma"
)

type Program struct {
	wasm       []byte
	progParams []byte
}

type Data struct {
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

func (p *Program) ProgParams() []byte {
	return p.progParams
}

func (s *Data) AddToHasher(hasher hash.Hash) {
	hasher.Write(s.bytes)
}

func (s *Data) Value() rma.SummaryValue {
	return rma.Uint64SummaryValue(0)
}

func (s *Data) Bytes() []byte {
	return s.bytes
}
