package clientmock

import (
	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

// MockAlphabillClient for testing. NOT thread safe.
type MockAlphabillClient struct {
	recordedTxs              []*txsystem.Transaction
	txResponse               *txsystem.TransactionResponse
	maxBlockNumber           uint64
	maxRoundNumber           uint64
	shutdown                 bool
	blocks                   map[uint64]*block.Block
	txListener               func(tx *txsystem.Transaction)
	incrementOnFetch         bool // if true, maxBlockNumber will be incremented on each GetBlocks call
	lastRequestedBlockNumber uint64
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
	if c.incrementOnFetch {
		defer c.SetMaxBlockNumber(blockNumber + 1)
	}
	if c.blocks != nil {
		b := c.blocks[blockNumber]
		return b, nil
	}
	return nil, nil
}

func (c *MockAlphabillClient) GetBlocks(blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
	c.lastRequestedBlockNumber = blockNumber
	if c.incrementOnFetch {
		defer c.SetMaxBlockNumber(blockNumber + 1)
	}
	batchMaxBlockNumber := blockNumber
	if blockNumber <= c.maxBlockNumber {
		var blocks []*block.Block
		b, f := c.blocks[blockNumber]
		if f {
			blocks = []*block.Block{b}
			batchMaxBlockNumber = b.UnicityCertificate.InputRecord.RoundNumber
		} else {
			blocks = []*block.Block{}
		}
		return &alphabill.GetBlocksResponse{
			MaxBlockNumber:      c.maxBlockNumber,
			MaxRoundNumber:      c.maxRoundNumber,
			Blocks:              blocks,
			BatchMaxBlockNumber: batchMaxBlockNumber,
		}, nil
	}
	return &alphabill.GetBlocksResponse{
		MaxBlockNumber:      c.maxBlockNumber,
		MaxRoundNumber:      c.maxRoundNumber,
		Blocks:              []*block.Block{},
		BatchMaxBlockNumber: batchMaxBlockNumber,
	}, nil
}

func (c *MockAlphabillClient) GetMaxBlockNumber() (uint64, uint64, error) {
	return c.maxBlockNumber, c.maxRoundNumber, nil
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
	if c.maxBlockNumber > c.maxRoundNumber {
		c.SetMaxRoundNumber(c.maxBlockNumber)
	}
}

func (c *MockAlphabillClient) SetMaxRoundNumber(roundNumber uint64) {
	if c.maxBlockNumber > roundNumber {
		panic("round number cannot be behind the block number")
	}
	c.maxRoundNumber = roundNumber
}

func (c *MockAlphabillClient) SetBlock(b *block.Block) {
	c.blocks[b.UnicityCertificate.InputRecord.RoundNumber] = b
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

func (c *MockAlphabillClient) SetIncrementOnFetch(incrementOnFetch bool) {
	c.incrementOnFetch = incrementOnFetch
}

func (c *MockAlphabillClient) GetLastRequestedBlockNumber() uint64 {
	return c.lastRequestedBlockNumber
}
