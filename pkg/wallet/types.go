package wallet

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type BlockProcessor interface {

	// ProcessBlock signals given block to be processesed
	// any error returned here signals block processor to terminate,
	ProcessBlock(b *block.Block) error
}

type TxHash []byte

type UnitID []byte

type Proof struct {
	BlockNumber uint64                `json:"blockNumber,string"`
	Tx          *txsystem.Transaction `json:"tx"`
	Proof       *block.BlockProof     `json:"proof"`
}

type Predicate []byte

type PubKey []byte
