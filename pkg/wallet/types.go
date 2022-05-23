package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
)

type (
	BlockProcessor interface {
		// BeginBlock signals the start of a new block.
		BeginBlock(blockNumber uint64) (BlockProcessorContext, error)
	}

	BlockProcessorContext interface {
		// ProcessTx processes the transaction. An error must be returned if the transaction processing was not successful.
		ProcessTx(tx *transaction.Transaction) error

		// EndBlock signals the end of the block.
		EndBlock() error

		// Rollback is called when any error occurs during block processing.
		Rollback() error

		// Commit is called when no error occurred during block processing.
		Commit() error
	}
)
