package partition

// BlockStore provides methods to store and query blockchain blocks.
type BlockStore interface {
	// Add adds the new block to the blockchain.
	Add(b *Block)
	// Height returns the number of committed blocks in the blockchain.
	Height() uint64
	// LatestBlock returns the latest committed block.
	LatestBlock() *Block
}
