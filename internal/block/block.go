package block

import (
	"crypto"
	"hash"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/mt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
)

// Hash returns the hash of the block.
func (x *Block) Hash(hashAlgorithm crypto.Hash) ([]byte, error) {
	// TODO refactor duplicate code
	hasher := hashAlgorithm.New()
	hasher.Write(x.SystemIdentifier)
	hasher.Write(util.Uint64ToBytes(x.BlockNumber))
	hasher.Write(x.PreviousBlockHash)

	txs := make([]mt.Data, len(x.Transactions))
	for i, tx := range x.Transactions {
		txBytes, err := tx.Bytes()
		if err != nil {
			return nil, err
		}
		txs[i] = &byteHasher{val: txBytes}
	}
	// build merkle tree of transactions
	merkleTree, err := mt.New(hashAlgorithm, txs)
	if err != nil {
		return nil, err
	}
	// add merkle tree root hash to block hasher
	hasher.Write(merkleTree.GetRootHash())

	return hasher.Sum(nil), nil
}

func (x *Block) HashHeader(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	x.AddHeaderToHasher(hasher)
	return hasher.Sum(nil)
}

func (x *Block) AddHeaderToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	// TODO add shard id to block header hash
	//hasher.Write(b.ShardIdentifier)
	hasher.Write(util.Uint64ToBytes(x.BlockNumber))
	hasher.Write(x.PreviousBlockHash)
}

// byteHasher helper struct to satisfy mt.Data interface
type byteHasher struct {
	val []byte
}

func (h *byteHasher) Hash(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(h.val)
	return hasher.Sum(nil)
}
