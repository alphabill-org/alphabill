package block

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

// unitHash creates unit hash for given primary and secondary unit transactions
func unitHash(primTx txsystem.GenericTransaction, secTxs []txsystem.GenericTransaction, hashAlgorithm crypto.Hash) ([]byte, error) {
	primhash := hashTx(primTx, hashAlgorithm)
	sechash, err := mt.SecondaryHash(secTxs, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return hash.Sum(hashAlgorithm, primhash, sechash), nil
}

// hashTx returns hash of given transaction or zero hash if nil transaction
func hashTx(tx txsystem.GenericTransaction, hashAlgorithm crypto.Hash) []byte {
	var h []byte
	hasher := hashAlgorithm.New()
	if tx != nil {
		hasher.Write(tx.ToProtoBuf().Bytes())
		h = hasher.Sum(nil)
	} else {
		h = make([]byte, hashAlgorithm.Size())
	}
	return h
}
