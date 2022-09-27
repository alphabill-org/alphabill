package block

import (
	"crypto"
	"hash"
	"sort"

	"github.com/alphabill-org/alphabill/internal/certificates"
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
	hasher := hashAlgorithm.New()
	hh := x.HashHeader(hashAlgorithm)
	hasher.Write(hh)

	txHasher := hashAlgorithm.New()
	x.AddTransactionsToHasher(txHasher)
	hasher.Write(txHasher.Sum(nil))

	leaves, err := x.BlockTreeLeaves(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	tree, err := omt.New(leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	hasher.Write(tree.GetRootHash())

	return hasher.Sum(nil), nil
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

func (x *GenericBlock) HashTransactions(hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	x.AddTransactionsToHasher(hasher)
	return hasher.Sum(nil)
}

func (x *GenericBlock) AddTransactionsToHasher(hasher hash.Hash) {
	for _, tx := range x.Transactions {
		hasher.Write(tx.ToProtoBuf().Bytes())
	}
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

// BlockTreeLeaves returns leaves for the ordered merkle tree
func (x *GenericBlock) BlockTreeLeaves(hashAlgorithm crypto.Hash) ([]*omt.Data, error) {
	leaves := make([]*omt.Data, len(x.Transactions))
	identifiers := x.ExtractIdentifiers()
	for i, unitId := range identifiers {
		primTx, secTxs := x.ExtractTransactions(unitId)
		unitHash, err := UnitHash(primTx, secTxs, hashAlgorithm)
		if err != nil {
			return nil, err
		}
		unitIdBytes := unitId.Bytes32()
		leaves[i] = &omt.Data{Val: unitIdBytes[:], Hash: unitHash}
	}
	return leaves, nil
}

// ExtractIdentifiers returns ordered list of unit ids for given transactions
func (x *GenericBlock) ExtractIdentifiers() []*uint256.Int {
	ids := make([]*uint256.Int, len(x.Transactions))
	for i, tx := range x.Transactions {
		ids[i] = tx.UnitID()
	}
	// sort ids in ascending order
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].Cmp(ids[j]) < 0
	})
	return ids
}

// ExtractTransactions returns primary tx and list of secondary txs for given unit
func (x *GenericBlock) ExtractTransactions(unitId *uint256.Int) (txsystem.GenericTransaction, []txsystem.GenericTransaction) {
	var primaryTx txsystem.GenericTransaction
	var secondaryTxs []txsystem.GenericTransaction
	for _, tx := range x.Transactions {
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
