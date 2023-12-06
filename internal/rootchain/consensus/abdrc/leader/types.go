package leader

import abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"

type BlockLoader func(round uint64) (*abtypes.BlockData, error)
