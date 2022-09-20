package proof

import (
	"bytes"
	"crypto"
	"sort"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/omt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
)

type TxTypeProvider interface {
	IsPrimary(tx *txsystem.Transaction) bool
}

// CreatePrimaryProof creates primary proof for given unit and block.
func CreatePrimaryProof(b *block.Block, unitId []byte, typeProvider TxTypeProvider, hashAlgorithm crypto.Hash) (*BlockProofV2, error) {
	if len(b.Transactions) == 0 {
		return newEmptyBlockProof(b, hashAlgorithm), nil
	}
	identifiers := extractIdentifiers(b.Transactions)
	leaves, err := blockTreeLeaves(b.Transactions, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	chain, err := treeChain(unitId, leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	if unitIdInIdentifiers(identifiers, unitId) {
		primTx, secTxs := extractTransactions(b.Transactions, unitId, typeProvider)
		secHash, err := secondaryHash(secTxs, hashAlgorithm)
		if err != nil {
			return nil, err
		}
		if primTx != nil {
			return newPrimBlockProof(b, secHash, chain, hashAlgorithm), nil
		}
		return newOnlysecBlockProof(b, secHash, chain, hashAlgorithm), nil
	}
	return newNotransBlockProof(b, chain, hashAlgorithm), nil
}

// VerifyProof returns true if given transaction and proof are valid.
func VerifyProof(tx *txsystem.Transaction, p *BlockProofV2, pubkey []byte, hashAlgorithm crypto.Hash) bool {
	// TODO verify UC
	switch p.ProofType {
	case ProofType_EMPTYBLOCK:
		return len(p.BlockTreeHashChain.Items) == 0
	case ProofType_SEC:
		// TODO impl
	case ProofType_ONLYSEC:
		// TODO impl
	case ProofType_PRIM:
		primhash := hashTx(tx, hashAlgorithm)
		unithash := hashUnit(primhash, p.HashValue, hashAlgorithm)
		return len(p.BlockTreeHashChain.Items) > 0 &&
			bytes.Equal(p.BlockTreeHashChain.Items[0].Val, tx.UnitId) &&
			bytes.Equal(p.BlockTreeHashChain.Items[0].Hash, unithash)
	case ProofType_NOTRANS:
		return len(p.BlockTreeHashChain.Items) > 0 && !bytes.Equal(p.BlockTreeHashChain.Items[0].Val, tx.UnitId)
	}
	return false
}

// ToProtobuf utility function that converts []mt.PathItem to proof.BlockMerkleProof
func ToProtobuf(srcPathItmes []*mt.PathItem) *BlockMerkleProof {
	dstPathItems := make([]*MerklePathItem, len(srcPathItmes))
	for i, srcPathItem := range srcPathItmes {
		dstPathItems[i] = &MerklePathItem{
			DirectionLeft: srcPathItem.DirectionLeft,
			PathItem:      srcPathItem.Hash,
		}
	}
	return &BlockMerkleProof{
		PathItems: dstPathItems,
	}
}

// FromProtobuf utility function that converts proof.BlockMerkleProof to []mt.PathItem
func FromProtobuf(proof *BlockMerkleProof) []*mt.PathItem {
	dstPathItems := make([]*mt.PathItem, len(proof.PathItems))
	for i, srcPathItem := range proof.PathItems {
		dstPathItems[i] = &mt.PathItem{
			Hash:          srcPathItem.PathItem,
			DirectionLeft: srcPathItem.DirectionLeft,
		}
	}
	return dstPathItems
}

func ToProtobufHashChain(chain []*omt.Data) []*ChainItem {
	r := make([]*ChainItem, len(chain))
	for i, c := range chain {
		r[i] = &ChainItem{Val: c.Val, Hash: c.Hash}
	}
	return r
}

// treeChain returns hash tree chain from given unit to root
func treeChain(unitId []byte, leaves []*omt.Data, hashAlgorithm crypto.Hash) ([]*omt.Data, error) {
	tree, err := omt.New(leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return tree.GetMerklePath(unitId)
}

// secondaryHash returns root merkle hash calculated from given txs
func secondaryHash(txs []*txsystem.Transaction, hashAlgorithm crypto.Hash) ([]byte, error) {
	// cast []*txsystem.Transaction to []mt.Data
	secTxs := make([]mt.Data, len(txs))
	for i, tx := range txs {
		gtx, err := txsystem.NewDefaultGenericTransaction(tx)
		if err != nil {
			return nil, err
		}
		secTxs[i] = gtx
	}
	// create merkle tree to get root hash
	tree, err := mt.New(hashAlgorithm, secTxs)
	if err != nil {
		return nil, err
	}
	return tree.GetRootHash(), nil
}

// extractTransactions returns ordered list of unit ids for given transactions
func extractIdentifiers(txs []*txsystem.Transaction) [][]byte {
	ids := make([][]byte, len(txs))
	for i, tx := range txs {
		ids[i] = tx.UnitId
	}
	// sort ids in ascending order
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i], ids[j]) < 0
	})
	return ids
}

// extractTransactions returns primary tx and list of secondary txs for given unit
func extractTransactions(txs []*txsystem.Transaction, unitId []byte, txTypeProvider TxTypeProvider) (*txsystem.Transaction, []*txsystem.Transaction) {
	var primaryTx *txsystem.Transaction
	var secondaryTxs []*txsystem.Transaction
	for _, tx := range txs {
		if bytes.Equal(tx.UnitId, unitId) {
			if txTypeProvider.IsPrimary(tx) {
				primaryTx = tx
			} else {
				secondaryTxs = append(secondaryTxs, tx)
			}
		}
	}
	return primaryTx, secondaryTxs
}

// blockTreeLeaves creates input for block tree
func blockTreeLeaves(txs []*txsystem.Transaction, hashAlgorithm crypto.Hash) ([]*omt.Data, error) {
	leaves := make([]*omt.Data, len(txs))
	identifiers := extractIdentifiers(txs)
	for i, unitId := range identifiers {
		primTx, secTxs := extractTransactions(txs, unitId, money.TxTypeProvider)
		hash, err := unitHash(primTx, secTxs, hashAlgorithm)
		if err != nil {
			return nil, err
		}
		leaves[i] = &omt.Data{Val: unitId, Hash: hash}
	}
	return leaves, nil
}

// unitHash creates unit hash for given primary and secondary unit transactions
func unitHash(primTx *txsystem.Transaction, secTxs []*txsystem.Transaction, hashAlgorithm crypto.Hash) ([]byte, error) {
	primhash := hashTx(primTx, hashAlgorithm)
	sechash, err := secondaryHash(secTxs, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return hashUnit(primhash, sechash, hashAlgorithm), nil
}

func hashUnit(primhash []byte, sechash []byte, hashAlgorithm crypto.Hash) []byte {
	hasher := hashAlgorithm.New()
	hasher.Write(primhash)
	hasher.Write(sechash)
	return hasher.Sum(nil)
}

// hashTx returns hash of given transaction or zero hash if nil transaction
func hashTx(tx *txsystem.Transaction, hashAlgorithm crypto.Hash) []byte {
	var hash []byte
	hasher := hashAlgorithm.New()
	if tx != nil {
		hasher.Write(tx.Bytes())
		hash = hasher.Sum(nil)
	} else {
		hash = make([]byte, hashAlgorithm.Size())
	}
	return hash
}

func newEmptyBlockProof(b *block.Block, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_EMPTYBLOCK,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          make([]byte, hashAlgorithm.Size()),
		BlockTreeHashChain: &BlockTreeHashChain{},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newNotransBlockProof(b *block.Block, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_NOTRANS,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          make([]byte, hashAlgorithm.Size()),
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newPrimBlockProof(b *block.Block, hashValue []byte, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_PRIM,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          hashValue,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newOnlysecBlockProof(b *block.Block, secHash []byte, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_ONLYSEC,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          secHash,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func unitIdInIdentifiers(items [][]byte, target []byte) bool {
	// TODO binary search, input is already sorted
	for _, item := range items {
		if bytes.Equal(item, target) {
			return true
		}
	}
	return false
}
