package wallet

import (
	"context"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type DummyBlockProcessor struct {
}

func (p DummyBlockProcessor) ProcessBlock(b *block.Block) error {
	return nil
}

type DummyAlphabillClient struct {
}

func (c *DummyAlphabillClient) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	return &txsystem.TransactionResponse{Ok: true}, nil
}

func (c *DummyAlphabillClient) GetBlock(blockNo uint64) (*block.Block, error) {
	return &block.Block{BlockNumber: blockNo}, nil
}

func (c *DummyAlphabillClient) GetBlocks(ctx context.Context, blockNumberFrom, blockNumberUntil uint64, ch chan<- *block.Block) error {
	ch <- &block.Block{BlockNumber: blockNumberFrom}
	return nil
}

func (c *DummyAlphabillClient) GetMaxBlockNumber() (uint64, error) {
	return 10, nil
}

func (c *DummyAlphabillClient) Shutdown() error {
	return nil
}

func (c *DummyAlphabillClient) IsShutdown() bool {
	return false
}
