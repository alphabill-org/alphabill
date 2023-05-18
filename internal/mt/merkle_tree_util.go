package mt

import (
	"crypto"
)

// SecondaryHash returns root merkle hash calculated from given txs
func SecondaryHash(txs []Data, hashAlgorithm crypto.Hash) ([]byte, error) {
	tree, err := New(hashAlgorithm, txs)
	if err != nil {
		return nil, err
	}
	return tree.GetRootHash(), nil
}

// SecondaryChain returns hash chain for a secondary transaction based on list of secondary transactions
func SecondaryChain(txs []Data, txIdx int, hashAlgorithm crypto.Hash) ([]*PathItem, error) {
	tree, err := New(hashAlgorithm, txs)
	if err != nil {
		return nil, err
	}
	return tree.GetMerklePath(txIdx)
}
