package block

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/omt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

var (
	ErrProofVerificationFailed = errors.New("proof verification failed")
	ErrTxIsNil                 = errors.New("tx is nil")
	ErrVerifierIsNil           = errors.New("verifier is nil")
	ErrBlockIsNil              = errors.New("block is nil")
	ErrUnitIdIsNil             = errors.New("unit id is nil")
)

// NewPrimaryProof creates primary proof for given unit and block.
func NewPrimaryProof(b *GenericBlock, unitId *uint256.Int, hashAlgorithm crypto.Hash) (*BlockProof, error) {
	if b == nil {
		return nil, ErrBlockIsNil
	}
	if unitId == nil {
		return nil, ErrUnitIdIsNil
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
		return nil, ErrBlockIsNil
	}
	if unitId == nil {
		return nil, ErrUnitIdIsNil
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
		return ErrTxIsNil
	}
	if verifiers == nil {
		return ErrVerifierIsNil
	}

	unitId := tx.UnitID()
	err := x.verifyUC(unitId, verifiers, hashAlgorithm)
	if err != nil {
		return err
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
		return ErrProofVerificationFailed
	case ProofType_EMPTYBLOCK:
		if len(x.getChainItems()) == 0 {
			return nil
		}
		return ErrProofVerificationFailed
	default:
		return aberrors.Wrap(ErrProofVerificationFailed, "unknown proof type "+x.ProofType.String())
	}
}

func (x *BlockProof) verifyUC(unitId *uint256.Int, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
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
		return aberrors.Wrap(
			ErrProofVerificationFailed,
			fmt.Sprintf("proof verification failed, uc.ir block hash is not valid, got %X, expected %X",
				x.UnicityCertificate.InputRecord.BlockHash, blockhash),
		)
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
	return ErrProofVerificationFailed
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
