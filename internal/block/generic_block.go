package block

import (
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
	BlockNumber        uint64
	PreviousBlockHash  []byte
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
	// TODO add shard id to block header hash
	//hasher.Write(b.ShardIdentifier)
	hasher.Write(util.Uint64ToBytes(x.BlockNumber))
	hasher.Write(x.PreviousBlockHash)
}

// ToProtobuf converts GenericBlock to protobuf Block
func (x *GenericBlock) ToProtobuf() *Block {
	return &Block{
		SystemIdentifier:   x.SystemIdentifier,
		BlockNumber:        x.BlockNumber,
		PreviousBlockHash:  x.PreviousBlockHash,
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
	type unitTxs struct {
		unitId *uint256.Int
		primTx txsystem.GenericTransaction
		secTxs []txsystem.GenericTransaction
	}
	res := make(map[uint256.Int]*unitTxs, len(x.Transactions))
	for _, tx := range x.Transactions {
		// create unitTx map entry if not exists already
		unitTx, exists := res[*tx.UnitID()]
		if !exists {
			unitTx = &unitTxs{
				unitId: tx.UnitID(),
			}
			res[*tx.UnitID()] = unitTx
		}

		// add tx to primary or secondary list
		if tx.IsPrimary() {
			unitTx.primTx = tx
		} else {
			unitTx.secTxs = append(unitTx.secTxs, tx)
		}
	}

	// get map values for sorting
	var vals []*unitTxs
	for _, v := range res {
		vals = append(vals, v)
	}

	// sort values by unit id
	sort.Slice(vals, func(i, j int) bool {
		return vals[i].unitId.Cmp(vals[j].unitId) < 0
	})

	// create leaves from sorted grouped transactions
	leaves := make([]*omt.Data, len(vals))
	for i, unitTx := range vals {
		h, err := unitHash(unitTx.primTx, unitTx.secTxs, hashAlgorithm)
		if err != nil {
			return nil, err
		}
		unitIdBytes := unitTx.unitId.Bytes32()
		leaves[i] = &omt.Data{Val: unitIdBytes[:], Hash: h}
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
func (x *GenericBlock) extractIdentifiers(hashAlgorithm crypto.Hash) []*uint256.Int {
	ids := make([]*uint256.Int, 0, len(x.Transactions))
	for _, tx := range x.Transactions {
		ids = append(ids, tx.TargetUnits(hashAlgorithm)...)
	}
	// sort ids in ascending order
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].Cmp(ids[j]) < 0
	})
	return ids
}

// extractTransactions returns primary tx and list of secondary txs for given unit
func (x *GenericBlock) extractTransactions(unitId *uint256.Int, hashAlgorithm crypto.Hash) (txsystem.GenericTransaction, []txsystem.GenericTransaction) {
	var primaryTx txsystem.GenericTransaction
	var secondaryTxs []txsystem.GenericTransaction
	for _, tx := range x.Transactions {
		for _, txUnitID := range tx.TargetUnits(hashAlgorithm) {
			if txUnitID.Eq(unitId) {
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
