package omt

import (
	"crypto"
	"sort"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

// BlockTreeLeaves returns leaves for the ordered merkle tree
func BlockTreeLeaves(txs []txsystem.GenericTransaction, hashAlgorithm crypto.Hash) ([]*Data, error) {
	leaves := make([]*Data, len(txs))
	identifiers := ExtractIdentifiers(txs)
	for i, unitId := range identifiers {
		primTx, secTxs := ExtractTransactions(txs, unitId)
		h, err := UnitHash(primTx, secTxs, hashAlgorithm)
		if err != nil {
			return nil, err
		}
		unitIdBytes := unitId.Bytes32()
		leaves[i] = &Data{Val: unitIdBytes[:], Hash: h}
	}
	return leaves, nil
}

// ExtractIdentifiers returns ordered list of unit ids for given transactions
func ExtractIdentifiers(txs []txsystem.GenericTransaction) []*uint256.Int {
	ids := make([]*uint256.Int, len(txs))
	for i, tx := range txs {
		ids[i] = tx.UnitID()
	}
	// sort ids in ascending order
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].Cmp(ids[j]) < 0
	})
	return ids
}

// ExtractTransactions returns primary tx and list of secondary txs for given unit
func ExtractTransactions(txs []txsystem.GenericTransaction, unitId *uint256.Int) (txsystem.GenericTransaction, []txsystem.GenericTransaction) {
	var primaryTx txsystem.GenericTransaction
	var secondaryTxs []txsystem.GenericTransaction
	for _, tx := range txs {
		if tx.UnitID().Eq(unitId) {
			if tx.IsPrimary() {
				primaryTx = tx
			} else {
				secondaryTxs = append(secondaryTxs, tx)
			}
		}
	}
	return primaryTx, secondaryTxs
}

// UnitHash creates unit hash for given primary and secondary unit transactions
func UnitHash(primTx txsystem.GenericTransaction, secTxs []txsystem.GenericTransaction, hashAlgorithm crypto.Hash) ([]byte, error) {
	primhash := HashTx(primTx, hashAlgorithm)
	sechash, err := mt.SecondaryHash(secTxs, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return hash.Sum(hashAlgorithm, primhash, sechash), nil
}

// HashTx returns hash of given transaction or zero hash if nil transaction
func HashTx(tx txsystem.GenericTransaction, hashAlgorithm crypto.Hash) []byte {
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
