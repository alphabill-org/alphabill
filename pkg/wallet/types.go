package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
)

type BlockProcessor interface {
	BeginBlock(blockNumber uint64) error
	ProcessTx(tx *transaction.Transaction) error
	EndBlock() error
}
