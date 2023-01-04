package block

import (
	"bytes"
	"crypto"
	"hash"
	"sort"

	"github.com/alphabill-org/alphabill/internal/certificates"
	abhash "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/omt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

// GenericBlock same as Block but transactions are of type txsystem.GenericTransaction
type GenericBlock struct {
	SystemIdentifier   []byte
	ShardIdentifier    []byte
	PreviousBlockHash  []byte
	NodeIdentifier     string
	Transactions       []txsystem.GenericTransaction
	UnicityCertificate *certificates.UnicityCertificate
}

// Hash returns the hash of the block.
// Hash is computed from hash of block header fields || hash of raw block payload || tree hash of transactions
func (x *GenericBlock) Hash(hashAlgorithm crypto.Hash) ([]byte, error) {
	headerHash := x.HashHeader(hashAlgorithm)
	txsHash := x.hashTransactions(hashAlgorithm)
	treeHash, err := x.treeHash(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return abhash.Sum(hashAlgorithm, headerHash, txsHash, treeHash), nil
}

func (x *GenericBlock) HashHeader(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	x.AddHeaderToHasher(hasher)
	return hasher.Sum(nil)
}

func (x *GenericBlock) AddHeaderToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	hasher.Write(x.ShardIdentifier)
	hasher.Write(x.PreviousBlockHash)
	hasher.Write([]byte(x.NodeIdentifier))
}

// ToProtobuf converts GenericBlock to protobuf Block
func (x *GenericBlock) ToProtobuf() *Block {
	return &Block{
		SystemIdentifier:   x.SystemIdentifier,
		ShardIdentifier:    x.ShardIdentifier,
		PreviousBlockHash:  x.PreviousBlockHash,
		NodeIdentifier:     x.NodeIdentifier,
		Transactions:       genericTxsToProtobuf(x.Transactions),
		UnicityCertificate: x.UnicityCertificate,
	}
}

func (x *GenericBlock) hashTransactions(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	x.addTransactionsToHasher(hasher)
	return hasher.Sum(nil)
}

func (x *GenericBlock) addTransactionsToHasher(hasher hash.Hash) {
	for _, tx := range x.Transactions {
		tx.AddToHasher(hasher)
	}
}

// blockTreeLeaves returns leaves for the ordered merkle tree
func (x *GenericBlock) blockTreeLeaves(hashAlgorithm crypto.Hash) ([]*omt.Data, error) {
	res := make(map[uint256.Int]*unitTxs, len(x.Transactions))
	for _, tx := range x.Transactions {
		unitIDs := tx.TargetUnits(hashAlgorithm)
		for _, unitID := range unitIDs {
			// create unitTx map entry if not exists already
			unitTx, exists := res[*unitID]
			if !exists {
				unitTx = &unitTxs{unitID: unitID}
				res[*unitID] = unitTx
			}

			// add tx to primary or secondary list
			if tx.IsPrimary() {
				unitTx.primTx = tx
			} else {
				unitTx.secTxs = append(unitTx.secTxs, tx)
			}
		}
	}

	// get map values for sorting
	var vals []*unitTxs
	for _, v := range res {
		vals = append(vals, v)
	}

	// sort values by unit id
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].unitID.Cmp(vals[j].unitID) < 0
	})

	// create leaves from sorted grouped transactions
	leaves := make([]*omt.Data, len(vals))
	for i, unitTx := range vals {
		h, err := unitTx.hash(hashAlgorithm)
		if err != nil {
			return nil, err
		}
		leaves[i] = &omt.Data{Val: util.Uint256ToBytes(unitTx.unitID), Hash: h}
	}
	return leaves, nil
}

// treeHash returns ordered merkle tree root hash for transactions
func (x *GenericBlock) treeHash(hashAlgorithm crypto.Hash) ([]byte, error) {
	leaves, err := x.blockTreeLeaves(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	tree, err := omt.New(leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return tree.GetRootHash(), nil
}

// extractIdentifiers returns ordered list of unit ids for given transactions
func (x *GenericBlock) extractIdentifiers(hashAlgorithm crypto.Hash) [][]byte {
	ids := make([][]byte, 0, len(x.Transactions))
	for _, tx := range x.Transactions {
		for _, unit := range tx.TargetUnits(hashAlgorithm) {
			ids = append(ids, util.Uint256ToBytes(unit))
		}
	}
	// sort ids in ascending order
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i], ids[j]) < 0
	})
	return ids
}

// extractTransactions returns primary tx and list of secondary txs for given unit
func (x *GenericBlock) extractTransactions(unitID []byte, hashAlgorithm crypto.Hash) (txsystem.GenericTransaction, []txsystem.GenericTransaction) {
	var primaryTx txsystem.GenericTransaction
	var secondaryTxs []txsystem.GenericTransaction
	unitID256 := uint256.NewInt(0).SetBytes(unitID)
	for _, tx := range x.Transactions {
		for _, txUnitID := range tx.TargetUnits(hashAlgorithm) {
			if txUnitID.Eq(unitID256) {
				if tx.IsPrimary() {
					primaryTx = tx
				} else {
					secondaryTxs = append(secondaryTxs, tx)
				}
			}
		}
	}
	return primaryTx, secondaryTxs
}

func genericTxsToProtobuf(src []txsystem.GenericTransaction) []*txsystem.Transaction {
	dst := make([]*txsystem.Transaction, len(src))
	for i, tx := range src {
		dst[i] = tx.ToProtoBuf()
	}
	return dst
}

func protobufTxsToGeneric(src []*txsystem.Transaction, txConverter TxConverter) ([]txsystem.GenericTransaction, error) {
	dst := make([]txsystem.GenericTransaction, len(src))
	var err error
	for i, tx := range src {
		dst[i], err = txConverter.ConvertTx(tx)
		if err != nil {
			return nil, err
		}
	}
	return dst, nil
}
