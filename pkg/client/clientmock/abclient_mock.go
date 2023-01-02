package clientmock

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

// MockAlphabillClient for testing. NOT thread safe.
type MockAlphabillClient struct {
	recordedTxs    []*txsystem.Transaction
	txResponse     *txsystem.TransactionResponse
	maxBlockNumber uint64
	shutdown       bool
	blocks         map[uint64]*block.Block
	txListener     func(tx *txsystem.Transaction)
}

func NewMockAlphabillClient(maxBlockNumber uint64, blocks map[uint64]*block.Block) *MockAlphabillClient {
	return &MockAlphabillClient{maxBlockNumber: maxBlockNumber, blocks: blocks}
}

func (c *MockAlphabillClient) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	c.recordedTxs = append(c.recordedTxs, tx)
	if c.txListener != nil {
		c.txListener(tx)
	}
	if c.txResponse != nil {
		if !c.txResponse.Ok {
			return c.txResponse, errors.New(c.txResponse.Message)
		}
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

func (c *MockAlphabillClient) GetBlocks(blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
	if blockNumber <= c.maxBlockNumber {
		var blocks []*block.Block
		b, f := c.blocks[blockNumber]
		if f {
			blocks = []*block.Block{b}
		} else {
			blocks = []*block.Block{}
		}
		return &alphabill.GetBlocksResponse{
			MaxBlockNumber: c.maxBlockNumber,
			Blocks:         blocks,
		}, nil
	}
	return &alphabill.GetBlocksResponse{
		MaxBlockNumber: c.maxBlockNumber,
		Blocks:         []*block.Block{},
	}, nil
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

func (c *MockAlphabillClient) SetBlock(b *block.Block) {
	c.blocks[b.BlockNumber] = b
}

func (c *MockAlphabillClient) GetRecordedTransactions() []*txsystem.Transaction {
	return c.recordedTxs
}

func (c *MockAlphabillClient) ClearRecordedTransactions() {
	c.recordedTxs = make([]*txsystem.Transaction, 0)
}

func (c *MockAlphabillClient) SetTxListener(txListener func(tx *txsystem.Transaction)) {
	c.txListener = txListener
}
