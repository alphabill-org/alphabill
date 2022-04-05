package partition

// BlockStore provides methods to store and query blockchain blocks.
type BlockStore interface {
	// Add adds the new block to the blockchain.
	Add(b *Block) error
	// Get returns the block with given number, or nil if not found.
	Get(blockNumber uint64) (*Block, error)
	// Height returns the number of committed blocks in the blockchain.
	Height() (uint64, error)
	// LatestBlock returns the latest committed block.
	LatestBlock() (*Block, error)
}
