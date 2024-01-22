package leader

import (
	"github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/storage"
)

type BlockLoader func(round uint64) (*storage.ExecutedBlock, error)
