package block

import (
	"crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/util"
)

// TODO AB-384
// Hash returns the hash of the block.
func (x *Block) Hash(hashAlgorithm crypto.Hash) ([]byte, error) {
	hasher := hashAlgorithm.New()
	x.AddHeaderToHasher(hasher)

	txs := make([]mt.Data, len(x.Transactions))
	for i, tx := range x.Transactions {
		txs[i] = &mt.ByteHasher{Val: tx.Bytes()}
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
