package partition

import (
	"crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

// Block is a set of transactions, grouped together for mostly efficiency reasons. At partition level a block is an
// ordered set of transactions, system identifier, reference to previous block hash, tx system block number, and
// unicity proofs.
// TODO idea: use protobuf instead?
type Block struct {
	systemIdentifier         []byte
	txSystemBlockNumber      uint64
	previousBlockHash        []byte
	transactions             []*transaction.Transaction // TODO use transaction struct/interface from AB-129
	UnicityCertificateRecord *UnicityCertificate
}

//Hash returns the hash of the block.
func (b Block) Hash(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(b.systemIdentifier)
	hasher.Write(transaction.Uint64ToBytes(b.txSystemBlockNumber))
	hasher.Write(b.previousBlockHash)
	// TODO continue implementing after task AB-129
	/*for _, tx := range b.transactions {
		tx.AddToHasher(hasher)
	}*/
	return hasher.Sum(nil)
}
