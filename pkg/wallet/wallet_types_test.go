package wallet

import (
	"github.com/alphabill-org/alphabill/internal/block"
)

type DummyBlockProcessor struct {
}

func (p DummyBlockProcessor) ProcessBlock(*block.Block) error {
	return nil
}
