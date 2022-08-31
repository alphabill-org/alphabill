package clientmock

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

// MockAlphabillClient for testing. NOT thread safe.
type MockAlphabillClient struct {
	recordedTxs    []*txsystem.Transaction
	txResponse     *txsystem.TransactionResponse
	maxBlockNumber uint64
	shutdown       bool
	blocks         map[uint64]*block.Block
}

func NewMockAlphabillClient(maxBlockNumber uint64, blocks map[uint64]*block.Block) *MockAlphabillClient {
	return &MockAlphabillClient{maxBlockNumber: maxBlockNumber, blocks: blocks}
}

func (c *MockAlphabillClient) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	c.recordedTxs = append(c.recordedTxs, tx)
	if c.txResponse != nil {
		return c.txResponse, nil
	}
	return &txsystem.TransactionResponse{Ok: true}, nil
}

func (c *MockAlphabillClient) GetBlock(blockNumber uint64) (*block.Block, error) {
	if c.blocks != nil {
		b := c.blocks[blockNumber]
		return b, nil
	}
	return nil, nil
}

func (c *MockAlphabillClient) GetBlocks(blockNumber, blockCount uint64) ([]*block.Block, error) {
	return []*block.Block{c.blocks[blockNumber]}, nil
}

func (c *MockAlphabillClient) GetMaxBlockNumber() (uint64, error) {
	return c.maxBlockNumber, nil
}

func (c *MockAlphabillClient) Shutdown() error {
	c.shutdown = true
	return nil
}

func (c *MockAlphabillClient) IsShutdown() bool {
	return c.shutdown
}

func (c *MockAlphabillClient) SetTxResponse(txResponse *txsystem.TransactionResponse) {
	c.txResponse = txResponse
}

func (c *MockAlphabillClient) SetMaxBlockNumber(blockNumber uint64) {
	c.maxBlockNumber = blockNumber
}

func (c *MockAlphabillClient) GetRecordedTransactions() []*txsystem.Transaction {
	return c.recordedTxs
}
