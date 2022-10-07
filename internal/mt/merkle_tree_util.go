package mt

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/txsystem"
)

// SecondaryHash returns root merkle hash calculated from given txs
func SecondaryHash(txs []txsystem.GenericTransaction, hashAlgorithm crypto.Hash) ([]byte, error) {
	tree, err := createTree(txs, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return tree.GetRootHash(), nil
}

// SecondaryChain returns hash chain for a secondary transaction based on list of secondary transactions
func SecondaryChain(txs []txsystem.GenericTransaction, txIdx int, hashAlgorithm crypto.Hash) ([]*PathItem, error) {
	tree, err := createTree(txs, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return tree.GetMerklePath(txIdx)
}

func createTree(txs []txsystem.GenericTransaction, hashAlgorithm crypto.Hash) (*MerkleTree, error) {
	// cast []txsystem.GenericTransaction to []mt.Data
	secTxs := make([]Data, len(txs))
	for i, tx := range txs {
		secTxs[i] = tx
	}
	// build tree
	return New(hashAlgorithm, secTxs)
}
