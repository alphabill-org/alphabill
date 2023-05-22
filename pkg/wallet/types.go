package wallet

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

type BlockProcessor interface {
	// ProcessBlock signals given block to be processesed
	// any error returned here signals block processor to terminate,
	ProcessBlock(b *types.Block) error
}

type TxHash []byte

type Transactions []*types.TransactionOrder

type UnitID []byte

type Proof struct {
	BlockNumber uint64                   `json:"blockNumber,string"`
	Tx          *types.TransactionRecord `json:"tx"`
	Proof       *types.TxProof           `json:"proof"`
}

type Predicate []byte

type PubKey []byte

// TxProof type alias for block.TxProof, can be removed once block package is moved out of internal
type TxProof = types.TxProof
