package wallet

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
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

func (c *DummyAlphabillClient) GetMaxBlockNumber() (uint64, error) {
	return 10, nil
}

func (c *DummyAlphabillClient) Shutdown() {
}

func (c *DummyAlphabillClient) IsShutdown() bool {
	return false
}
