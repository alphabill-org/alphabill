package wallet

import (
	"github.com/alphabill-org/alphabill/internal/block"
)

type BlockProcessor interface {

	// ProcessBlock signals given block to be processesed
	// any error returned here signals block processor to terminate,
	ProcessBlock(b *block.Block) error
}
