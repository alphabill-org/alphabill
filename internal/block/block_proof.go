package block

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	gohash "hash"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/omt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrProofVerificationFailed = errors.New("proof verification failed")
	ErrTxIsNil                 = errors.New("tx is nil")
	ErrVerifierIsNil           = errors.New("verifier is nil")
	ErrBlockIsNil              = errors.New("block is nil")
	ErrInvalidUnitID           = errors.New("unit id is invalid")
	ErrUCIsNil                 = errors.New("unicity certificate is nil")
)

// NewPrimaryProof creates primary proof for given unit and block.
func NewPrimaryProof(b *GenericBlock, unitID []byte, hashAlgorithm crypto.Hash) (*BlockProof, error) {
	if b == nil {
		return nil, ErrBlockIsNil
	}
	if len(unitID) == 0 {
		return nil, ErrInvalidUnitID
	}

	if len(b.Transactions) == 0 {
		return newEmptyBlockProof(b, hashAlgorithm), nil
	}
	identifiers := b.extractIdentifiers(hashAlgorithm)
	leaves, err := b.blockTreeLeaves(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	chain, err := treeChain(unitID, leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	if unitIDInIdentifiers(identifiers, unitID) {
		primTx, secTxs := b.extractTransactions(unitID, hashAlgorithm)
		secHash, err := mt.SecondaryHash(convertTxs(secTxs), hashAlgorithm)
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
func NewSecondaryProof(b *GenericBlock, unitID []byte, secTxIdx int, hashAlgorithm crypto.Hash) (*BlockProof, error) {
	if b == nil {
		return nil, ErrBlockIsNil
	}
	if len(unitID) != 32 {
		return nil, ErrInvalidUnitID
	}

	if len(b.Transactions) == 0 {
		return newEmptyBlockProof(b, hashAlgorithm), nil
	}
	leaves, err := b.blockTreeLeaves(hashAlgorithm)
	if err != nil {
		return nil, err
	}
	chain, err := treeChain(unitID, leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	primTx, secTxs := b.extractTransactions(unitID, hashAlgorithm)
	primhash := hashTx(primTx, hashAlgorithm)
	secChain, err := mt.SecondaryChain(convertTxs(secTxs), secTxIdx, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return newSecBlockProof(b, primhash, chain, secChain, hashAlgorithm), nil
}

// Verify verifies the proof against given transaction, returns error if verification failed, or nil if verification succeeded.
func (x *TxProof) Verify(unitID []byte, tx txsystem.GenericTransaction, verifiers map[string]abcrypto.Verifier, hashAlgo crypto.Hash) error {
	return x.Proof.Verify(unitID, tx, verifiers, hashAlgo)
}

// Verify verifies the proof against given transaction, returns error if verification failed, or nil if verification succeeded.
func (x *BlockProof) Verify(unitID []byte, tx txsystem.GenericTransaction, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	if len(unitID) == 0 {
		return ErrInvalidUnitID
	}
	if verifiers == nil {
		return ErrVerifierIsNil
	}
	err := x.verifyUC(unitID, verifiers, hashAlgorithm)
	if err != nil {
		return err
	}
	switch x.ProofType {
	case ProofType_PRIM:
		if tx == nil {
			return ErrTxIsNil
		}
		primhash := hashTx(tx, hashAlgorithm)
		unithash := hash.Sum(hashAlgorithm, primhash, x.HashValue)
		return x.verifyChainHead(unitID, unithash)
	case ProofType_SEC:
		if tx == nil {
			return ErrTxIsNil
		}
		secChain := FromProtobuf(x.SecTreeHashChain.Items)
		secChainOutput := mt.EvalMerklePath(secChain, tx, hashAlgorithm)
		unithash := hash.Sum(hashAlgorithm, x.HashValue, secChainOutput)
		return x.verifyChainHead(unitID, unithash)
	case ProofType_ONLYSEC:
		zerohash := make([]byte, hashAlgorithm.Size())
		unithash := hash.Sum(hashAlgorithm, zerohash, x.HashValue)
		return x.verifyChainHead(unitID, unithash)
	case ProofType_NOTRANS:
		chain := x.getChainItems()
		if len(chain) > 0 && !bytes.Equal(chain[0].Val, unitID) {
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

func (x *BlockProof) Bytes() []byte {
	var b bytes.Buffer
	proofType := ProofType_value[x.ProofType.String()]
	b.Write(util.Uint32ToBytes(uint32(proofType)))
	b.Write(x.BlockHeaderHash)
	b.Write(x.TransactionsHash)
	b.Write(x.HashValue)
	if x.BlockTreeHashChain != nil {
		x.BlockTreeHashChain.addToBuffer(&b)
	}
	if x.SecTreeHashChain != nil {
		x.SecTreeHashChain.addToBuffer(&b)
	}
	if x.UnicityCertificate != nil {
		b.Write(x.UnicityCertificate.Bytes())
	}
	return b.Bytes()
}

func (x *BlockProof) AddToHasher(hasher gohash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *BlockProof) verifyUC(unitID []byte, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	if x.UnicityCertificate == nil {
		return ErrUCIsNil
	}
	sysid := x.UnicityCertificate.UnicityTreeCertificate.SystemIdentifier
	sdr := x.UnicityCertificate.UnicityTreeCertificate.SystemDescriptionHash
	err := x.UnicityCertificate.IsValid(verifiers, hashAlgorithm, sysid, sdr)
	if err != nil {
		return err
	}

	chain := FromProtobufHashChain(x.getChainItems())
	rblock := omt.EvalMerklePath(chain, unitID, hashAlgorithm)
	blockhash, err := makeBlockHash(hashAlgorithm, len(chain) != 0, func() (*genericBlockHashingContext, error) {
		return &genericBlockHashingContext{
			headerHash: x.BlockHeaderHash,
			txsHash:    x.TransactionsHash,
			treeHash:   rblock,
		}, nil
	})
	if err != nil {
		return err
	}
	if !bytes.Equal(x.UnicityCertificate.InputRecord.BlockHash, blockhash) {
		return aberrors.Wrap(
			ErrProofVerificationFailed,
			fmt.Sprintf("proof verification failed, uc.ir block hash is not valid, got %X, expected %X",
				x.UnicityCertificate.InputRecord.BlockHash, blockhash),
		)
	}
	return nil
}

func (x *BlockProof) verifyChainHead(unitID []byte, unithash []byte) error {
	chain := x.getChainItems()
	if len(chain) > 0 &&
		bytes.Equal(chain[0].Val, unitID) &&
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
func treeChain(unitID []byte, leaves []*omt.Data, hashAlgorithm crypto.Hash) ([]*omt.Data, error) {
	tree, err := omt.New(leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return tree.GetMerklePath(unitID)
}

func unitIDInIdentifiers(items [][]byte, target []byte) bool {
	low := 0
	high := len(items) - 1
	for low <= high {
		mid := low + (high-low)/2
		midItem := items[mid]
		comp := bytes.Compare(midItem, target)
		if comp < 0 {
			low = mid + 1
		} else if comp > 0 {
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

func (x *BlockTreeHashChain) addToBuffer(b *bytes.Buffer) {
	for _, item := range x.Items {
		item.addToBuffer(b)
	}
}

func (x *SecTreeHashChain) addToBuffer(b *bytes.Buffer) {
	for _, item := range x.Items {
		item.addToBuffer(b)
	}
}

func (x *ChainItem) addToBuffer(b *bytes.Buffer) {
	b.Write(x.Val)
	b.Write(x.Hash)
}

func (x *MerklePathItem) addToBuffer(b *bytes.Buffer) {
	if x.DirectionLeft {
		b.WriteByte(1)
	} else {
		b.WriteByte(0)
	}
	b.Write(x.PathItem)
}
