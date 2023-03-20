package wallet

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type DummyBlockProcessor struct {
}

func (p DummyBlockProcessor) ProcessBlock(b *block.Block) error {
	return nil
}

type DummyAlphabillClient struct {
}

func (c *DummyAlphabillClient) SendTransaction(ctx context.Context, tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	return &txsystem.TransactionResponse{Ok: true}, nil
}

func (c *DummyAlphabillClient) GetBlock(ctx context.Context, blockNo uint64) (*block.Block, error) {
	return &block.Block{BlockNumber: blockNo}, nil
}

func (c *DummyAlphabillClient) GetBlocks(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
	return &alphabill.GetBlocksResponse{MaxBlockNumber: 10, Blocks: []*block.Block{{BlockNumber: blockNumber}}}, nil
}

func (c *DummyAlphabillClient) GetMaxBlockNumber(ctx context.Context) (uint64, error) {
	return 10, nil
}

func (c *DummyAlphabillClient) Shutdown() error {
	return nil
}

func (c *DummyAlphabillClient) IsShutdown() bool {
	return false
}
