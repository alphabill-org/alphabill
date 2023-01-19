package wallet

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
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

func (c *DummyAlphabillClient) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	return &txsystem.TransactionResponse{Ok: true}, nil
}

func (c *DummyAlphabillClient) GetBlock(blockNo uint64) (*block.Block, error) {
	return &block.Block{UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: blockNo}}}, nil
}

func (c *DummyAlphabillClient) GetBlocks(blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
	return &alphabill.GetBlocksResponse{MaxBlockNumber: 10, Blocks: []*block.Block{{UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: blockNumber}}}}}, nil
}

func (c *DummyAlphabillClient) GetMaxBlockNumber() (uint64, uint64, error) {
	return 10, 10, nil
}

func (c *DummyAlphabillClient) Shutdown() error {
	return nil
}

func (c *DummyAlphabillClient) IsShutdown() bool {
	return false
}
