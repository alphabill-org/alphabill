package block

import (
	"bytes"
	"crypto"
	"errors"
	gohash "hash"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/omt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

const (
	ErrProofVerificationFailed = "proof verification failed"
	ErrTxIsNil                 = "tx is nil"
	ErrVerifierIsNil           = "verifier is nil"
	ErrBlockIsNil              = "block is nil"
	ErrUnitIdIsNil             = "unit id is nil"
)

// NewPrimaryProof creates primary proof for given unit and block.
func NewPrimaryProof(b *GenericBlock, unitId *uint256.Int, hashAlgorithm crypto.Hash) (*BlockProof, error) {
	if b == nil {
		return nil, errors.New(ErrBlockIsNil)
	}
	if unitId == nil {
		return nil, errors.New(ErrUnitIdIsNil)
	}

	if len(b.Transactions) == 0 {
		return newEmptyBlockProof(b, hashAlgorithm), nil
	}
	identifiers := b.extractIdentifiers()
	leaves, err := b.blockTreeLeaves(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	chain, err := treeChain(unitId, leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	if unitIdInIdentifiers(identifiers, unitId) {
		primTx, secTxs := b.extractTransactions(unitId)
		secHash, err := mt.SecondaryHash(secTxs, hashAlgorithm)
		if err != nil {
			return nil, err
		}
		if primTx != nil {
			return newPrimBlockProof(b, secHash, chain, hashAlgorithm), nil
		}
		return newOnlySecBlockProof(b, secHash, chain, hashAlgorithm), nil
	}
	return newNoTransBlockProof(b, chain, hashAlgorithm), nil
}

// NewSecondaryProof creates secondary proof for given unit and block.
func NewSecondaryProof(b *GenericBlock, unitId *uint256.Int, secTxIdx int, hashAlgorithm crypto.Hash) (*BlockProof, error) {
	if b == nil {
		return nil, errors.New(ErrBlockIsNil)
	}
	if unitId == nil {
		return nil, errors.New(ErrUnitIdIsNil)
	}

	if len(b.Transactions) == 0 {
		return newEmptyBlockProof(b, hashAlgorithm), nil
	}
	leaves, err := b.blockTreeLeaves(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	chain, err := treeChain(unitId, leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	primTx, secTxs := b.extractTransactions(unitId)
	primhash := hashTx(primTx, hashAlgorithm)
	secChain, err := mt.SecondaryChain(secTxs, secTxIdx, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return newSecBlockProof(b, primhash, chain, secChain, hashAlgorithm), nil
}

// Verify verifies the proof against given transaction, returns error if verification failed, or nil if verification succeeded.
func (x *BlockProof) Verify(tx txsystem.GenericTransaction, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	if tx == nil {
		return errors.New(ErrTxIsNil)
	}
	if verifiers == nil {
		return errors.New(ErrVerifierIsNil)
	}

	unitId := tx.UnitID()
	err := x.verifyUC(unitId, verifiers, hashAlgorithm)
	if err != nil {
		return aberrors.Wrap(err, "block proof UC verification failed")
	}

	switch x.ProofType {
	case ProofType_PRIM:
		primhash := hashTx(tx, hashAlgorithm)
		unithash := hash.Sum(hashAlgorithm, primhash, x.HashValue)
		return x.verifyChainHead(unitId, unithash)
	case ProofType_SEC:
		secChain := FromProtobuf(x.SecTreeHashChain.Items)
		secChainOutput := mt.EvalMerklePath(secChain, tx, hashAlgorithm)
		unithash := hash.Sum(hashAlgorithm, x.HashValue, secChainOutput)
		return x.verifyChainHead(unitId, unithash)
	case ProofType_ONLYSEC:
		zerohash := make([]byte, hashAlgorithm.Size())
		unithash := hash.Sum(hashAlgorithm, zerohash, x.HashValue)
		return x.verifyChainHead(unitId, unithash)
	case ProofType_NOTRANS:
		unitIdBytes := unitId.Bytes32()
		chain := x.getChainItems()
		if len(chain) > 0 && !bytes.Equal(chain[0].Val, unitIdBytes[:]) {
			return nil
		}
		return errors.New(ErrProofVerificationFailed)
	case ProofType_EMPTYBLOCK:
		if len(x.getChainItems()) == 0 {
			return nil
		}
		return errors.New(ErrProofVerificationFailed)
	default:
		return aberrors.New(ErrProofVerificationFailed + " unknown proof type " + x.ProofType.String())
	}
}

func (x *BlockProof) Bytes() []byte {
	var b bytes.Buffer
	proofType := ProofType_value[x.ProofType.String()]
	b.Write(util.Uint32ToBytes(uint32(proofType)))
	b.Write(x.BlockHeaderHash)
	b.Write(x.TransactionsHash)
	b.Write(x.HashValue)
	if x.BlockTreeHashChain != nil {
		x.BlockTreeHashChain.addToBuffer(b)
	}
	if x.SecTreeHashChain != nil {
		x.SecTreeHashChain.addToBuffer(b)
	}
	if x.UnicityCertificate != nil {
		b.Write(x.UnicityCertificate.Bytes())
	}
	return b.Bytes()
}

func (x *BlockProof) AddToHasher(hasher gohash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *BlockProof) verifyUC(unitId *uint256.Int, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	if x.UnicityCertificate == nil {
		return errors.New(ErrProofVerificationFailed + " unicity certificate is nil")
	}
	sysid := x.UnicityCertificate.UnicityTreeCertificate.SystemIdentifier
	sdr := x.UnicityCertificate.UnicityTreeCertificate.SystemDescriptionHash
	err := x.UnicityCertificate.IsValid(verifiers, hashAlgorithm, sysid, sdr)
	if err != nil {
		return err
	}

	chain := FromProtobufHashChain(x.getChainItems())
	unitIdBytes := unitId.Bytes32()
	rblock := omt.EvalMerklePath(chain, unitIdBytes[:], hashAlgorithm)
	blockhash := hash.Sum(hashAlgorithm, x.BlockHeaderHash, x.TransactionsHash, rblock)
	if !bytes.Equal(x.UnicityCertificate.InputRecord.BlockHash, blockhash) {
		return aberrors.Errorf("proof verification failed, uc.ir block hash is not valid, got %X, expected %X",
			x.UnicityCertificate.InputRecord.BlockHash, blockhash)
	}
	return nil
}

func (x *BlockProof) verifyChainHead(unitId *uint256.Int, unithash []byte) error {
	chain := x.getChainItems()
	unitIdBytes := unitId.Bytes32()
	if len(chain) > 0 &&
		bytes.Equal(chain[0].Val, unitIdBytes[:]) &&
		bytes.Equal(chain[0].Hash, unithash) {
		return nil
	}
	return aberrors.New(ErrProofVerificationFailed)
}

// getChainItems returns BlockTreeHashChain chain items, used for nil safe access.
func (x *BlockProof) getChainItems() []*ChainItem {
	chain := x.BlockTreeHashChain
	if chain != nil {
		return chain.Items
	}
	return nil
}

// treeChain returns hash tree chain from given unit to root
func treeChain(unitId *uint256.Int, leaves []*omt.Data, hashAlgorithm crypto.Hash) ([]*omt.Data, error) {
	tree, err := omt.New(leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	unitIdBytes := unitId.Bytes32()
	return tree.GetMerklePath(unitIdBytes[:])
}

func unitIdInIdentifiers(items []*uint256.Int, target *uint256.Int) bool {
	low := 0
	high := len(items) - 1
	for low <= high {
		mid := low + (high-low)/2
		midItem := items[mid]
		if midItem.Lt(target) {
			low = mid + 1
		} else if midItem.Gt(target) {
			high = mid - 1
		} else {
			return true
		}
	}
	return false
}

func newEmptyBlockProof(b *GenericBlock, hashAlgorithm crypto.Hash) *BlockProof {
	return &BlockProof{
		ProofType:          ProofType_EMPTYBLOCK,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		TransactionsHash:   b.hashTransactions(hashAlgorithm),
		HashValue:          make([]byte, hashAlgorithm.Size()),
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newNoTransBlockProof(b *GenericBlock, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProof {
	return &BlockProof{
		ProofType:          ProofType_NOTRANS,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		TransactionsHash:   b.hashTransactions(hashAlgorithm),
		HashValue:          make([]byte, hashAlgorithm.Size()),
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newPrimBlockProof(b *GenericBlock, hashValue []byte, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProof {
	return &BlockProof{
		ProofType:          ProofType_PRIM,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		TransactionsHash:   b.hashTransactions(hashAlgorithm),
		HashValue:          hashValue,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newOnlySecBlockProof(b *GenericBlock, secHash []byte, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProof {
	return &BlockProof{
		ProofType:          ProofType_ONLYSEC,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		TransactionsHash:   b.hashTransactions(hashAlgorithm),
		HashValue:          secHash,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newSecBlockProof(b *GenericBlock, secHash []byte, chain []*omt.Data, secChain []*mt.PathItem, hashAlgorithm crypto.Hash) *BlockProof {
	return &BlockProof{
		ProofType:          ProofType_SEC,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		TransactionsHash:   b.hashTransactions(hashAlgorithm),
		HashValue:          secHash,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		SecTreeHashChain:   &SecTreeHashChain{Items: ToProtobuf(secChain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func (x *BlockTreeHashChain) addToBuffer(b bytes.Buffer) {
	for _, item := range x.Items {
		item.addToBuffer(b)
	}
}

func (x *SecTreeHashChain) addToBuffer(b bytes.Buffer) {
	for _, item := range x.Items {
		item.addToBuffer(b)
	}
}

func (x *ChainItem) addToBuffer(b bytes.Buffer) {
	b.Write(x.Val)
	b.Write(x.Hash)
}

func (x *MerklePathItem) addToBuffer(b bytes.Buffer) {
	if x.DirectionLeft {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
	}
	b.Write(x.PathItem)
}
