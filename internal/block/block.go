package block

import (
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
)

//Hash returns the hash of the block.
func (x *Block) Hash(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	x.AddHeaderToHasher(hasher)
	// TODO continue implementing after task AB-129
	/*for _, tx := range b.Transactions {
		tx.AddToHasher(hasher)
	}*/
	return hasher.Sum(nil)
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
