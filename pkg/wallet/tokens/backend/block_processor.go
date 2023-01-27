package twb

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
)

type blockProcessor struct {
	store Storage
	txp   *txProcessor
}

func (p *blockProcessor) ProcessBlock(ctx context.Context, b *block.Block) error {
	lastBlockNumber, err := p.store.GetBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to read current block number: %w", err)
	}
	if b.BlockNumber != lastBlockNumber+1 {
		return fmt.Errorf("invalid block height. Received blockNumber %d current wallet blockNumber %d", b.BlockNumber, lastBlockNumber)
	}

	for _, tx := range b.Transactions {
		if err := p.txp.readTx(tx, b); err != nil {
			return fmt.Errorf("failed to process tx: %w", err)
		}
	}

	return p.store.SetBlockNumber(b.BlockNumber)
}
