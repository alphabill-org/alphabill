package block

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/util"
)

//Hash returns the hash of the block.
func (x *Block) Hash(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(x.SystemIdentifier)
	hasher.Write(util.Uint64ToBytes(x.BlockNumber))
	hasher.Write(x.PreviousBlockHash)
	// TODO continue implementing after task AB-129
	/*for _, tx := range b.Transactions {
		tx.AddToHasher(hasher)
	}*/
	return hasher.Sum(nil)
}
