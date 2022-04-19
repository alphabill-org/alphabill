package block

import (
	"crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
)

// Block is a set of transactions, grouped together for mostly efficiency reasons. At partition level a block is an
// ordered set of transactions, system identifier, reference to previous block hash, tx system block number, and
// unicity proofs.
// TODO idea: use protobuf instead?
type Block struct {
	SystemIdentifier    []byte                           `json:"systemIdentifier"`
	TxSystemBlockNumber uint64                           `json:"txSystemBlockNumber"`
	PreviousBlockHash   []byte                           `json:"previousBlockHash"`
	Transactions        []*transaction.Transaction       `json:"transactions"` // TODO use transaction struct/interface from AB-129
	UnicityCertificate  *certificates.UnicityCertificate `json:"unicityCertificate"`
}

//Hash returns the hash of the block.
func (b Block) Hash(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(b.SystemIdentifier)
	hasher.Write(util.Uint64ToBytes(b.TxSystemBlockNumber))
	hasher.Write(b.PreviousBlockHash)
	// TODO continue implementing after task AB-129
	/*for _, tx := range b.Transactions {
		tx.AddToHasher(hasher)
	}*/
	return hasher.Sum(nil)
}
