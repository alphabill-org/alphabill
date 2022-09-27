package block

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

// UnitHash creates unit hash for given primary and secondary unit transactions
func UnitHash(primTx txsystem.GenericTransaction, secTxs []txsystem.GenericTransaction, hashAlgorithm crypto.Hash) ([]byte, error) {
	primhash := HashTx(primTx, hashAlgorithm)
	sechash, err := mt.SecondaryHash(secTxs, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return HashData(primhash, sechash, hashAlgorithm), nil
}

// HashData hashes together two arbitary data units
func HashData(h1 []byte, h2 []byte, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(h1)
	hasher.Write(h2)
	return hasher.Sum(nil)
}

// HashTx returns hash of given transaction or zero hash if nil transaction
func HashTx(tx txsystem.GenericTransaction, hashAlgorithm crypto.Hash) []byte {
	var hash []byte
	hasher := hashAlgorithm.New()
	if tx != nil {
		hasher.Write(tx.ToProtoBuf().Bytes())
		hash = hasher.Sum(nil)
	} else {
		hash = make([]byte, hashAlgorithm.Size())
	}
	return hash
}
