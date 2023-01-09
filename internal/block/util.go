package block

import (
	"crypto"

	abhash "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

// unitTxs helper struct for optimizing block tree leaves calculation
type unitTxs struct {
	unitID *uint256.Int
	primTx txsystem.GenericTransaction
	secTxs []txsystem.GenericTransaction
}

func (u *unitTxs) hash(hashAlgorithm crypto.Hash) ([]byte, error) {
	primhash := hashTx(u.primTx, hashAlgorithm)
	sechash, err := mt.SecondaryHash(convertTxs(u.secTxs), hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return abhash.Sum(hashAlgorithm, primhash, sechash), nil
}

// hashTx returns hash of given transaction or zero hash if nil transaction
func hashTx(tx txsystem.GenericTransaction, hashAlgorithm crypto.Hash) []byte {
	var h []byte
	if tx != nil {
		h = tx.Hash(hashAlgorithm)
	} else {
		h = make([]byte, hashAlgorithm.Size())
	}
	return h
}

func convertTxs(txs []txsystem.GenericTransaction) []mt.Data {
	// cast []txsystem.GenericTransaction to []mt.Data
	res := make([]mt.Data, len(txs))
	for i, tx := range txs {
		res[i] = tx
	}
	return res
}
