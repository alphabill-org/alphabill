package proof

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/mt"
	"github.com/alphabill-org/alphabill/internal/omt"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

// NewPrimaryProof creates primary proof for given unit and block.
func NewPrimaryProof(b *block.GenericBlock, unitId *uint256.Int, hashAlgorithm crypto.Hash) (*BlockProofV2, error) {
	if len(b.Transactions) == 0 {
		return newEmptyBlockProof(b, hashAlgorithm), nil
	}
	identifiers := omt.ExtractIdentifiers(b.Transactions)
	leaves, err := omt.BlockTreeLeaves(b.Transactions, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	chain, err := treeChain(unitId, leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	if unitIdInIdentifiers(identifiers, unitId) {
		primTx, secTxs := omt.ExtractTransactions(b.Transactions, unitId)
		secHash, err := mt.SecondaryHash(secTxs, hashAlgorithm)
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

// NewSecondaryProof creates secondary proof for given unit and block.
func NewSecondaryProof(b *block.GenericBlock, unitId *uint256.Int, secTxIdx int, hashAlgorithm crypto.Hash) (*BlockProofV2, error) {
	if len(b.Transactions) == 0 {
		return newEmptyBlockProof(b, hashAlgorithm), nil
	}
	leaves, err := omt.BlockTreeLeaves(b.Transactions, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	chain, err := treeChain(unitId, leaves, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	primTx, secTxs := omt.ExtractTransactions(b.Transactions, unitId)
	primhash := omt.HashTx(primTx, hashAlgorithm)
	secChain, err := mt.SecondaryChain(secTxs, secTxIdx, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return newSecBlockProof(b, primhash, chain, secChain, hashAlgorithm), nil
}

// Verify verifies the proof against given transaction, returns error if verification failed, or nil if verification succeeded.
func (x *BlockProofV2) Verify(tx txsystem.GenericTransaction, verifier abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	err := verifyUC(tx, x, verifier, hashAlgorithm)
	if err != nil {
		return err
	}

	switch x.ProofType {
	case ProofType_PRIM:
		primhash := omt.HashTx(tx, hashAlgorithm)
		unithash := omt.HashData(primhash, x.HashValue, hashAlgorithm)
		if x.verifyChainHead(tx.UnitID(), unithash) {
			return nil
		}
		return errors.New("PRIM proof verification failed")
	case ProofType_SEC:
		secChain := FromProtobuf(x.SecTreeHashChain.Items)
		secChainOutput := mt.EvalMerklePath(secChain, tx, hashAlgorithm)
		unithash := omt.HashData(x.HashValue, secChainOutput, hashAlgorithm)
		if x.verifyChainHead(tx.UnitID(), unithash) {
			return nil
		}
		return errors.New("SEC proof verification failed")
	case ProofType_ONLYSEC:
		// TODO impl
		return nil
	case ProofType_NOTRANS:
		unitIdBytes := tx.UnitID().Bytes32()
		chain := x.BlockTreeHashChain.Items
		if len(chain) > 0 && !bytes.Equal(chain[0].Val, unitIdBytes[:]) {
			return nil
		}
		return errors.New("NOTRANS proof verification failed")
	case ProofType_EMPTYBLOCK:
		if len(x.BlockTreeHashChain.Items) == 0 {
			return nil
		}
		return errors.New("EMPTYBLOCK proof verification failed")
	default:
		return errors.New("proof verification failed, unknown proof type " + x.ProofType.String())
	}
}

func (x *BlockProofV2) verifyChainHead(unitId *uint256.Int, unithash []byte) bool {
	chain := x.BlockTreeHashChain.Items
	unitIdBytes := unitId.Bytes32()
	return len(chain) > 0 &&
		bytes.Equal(chain[0].Val, unitIdBytes[:]) &&
		bytes.Equal(chain[0].Hash, unithash)
}

// verifyUC verifies unicity certificate and uc.ir.blockhash
func verifyUC(tx txsystem.GenericTransaction, p *BlockProofV2, verifier abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	sysid := p.UnicityCertificate.UnicityTreeCertificate.SystemIdentifier
	sdr := p.UnicityCertificate.UnicityTreeCertificate.SystemDescriptionHash
	err := p.UnicityCertificate.IsValid(verifier, hashAlgorithm, sysid, sdr)
	if err != nil {
		return err
	}

	chain := FromProtobufHashChain(p.BlockTreeHashChain.Items)
	unitIdBytes := tx.UnitID().Bytes32()
	rblock := omt.EvalMerklePath(chain, unitIdBytes[:], hashAlgorithm)
	hasher := hashAlgorithm.New()
	hasher.Write(p.BlockHeaderHash)
	hasher.Write(rblock)
	blockhash := hasher.Sum(nil)
	if !bytes.Equal(p.UnicityCertificate.InputRecord.BlockHash, blockhash) {
		return errors.New(fmt.Sprintf("proof verification failed, uc.ir block hash is not valid, got %X, expected %X",
			p.UnicityCertificate.InputRecord.BlockHash, blockhash))
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
	// TODO binary search, input is already sorted
	for _, item := range items {
		if item.Eq(target) {
			return true
		}
	}
	return false
}

func newEmptyBlockProof(b *block.GenericBlock, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_EMPTYBLOCK,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          make([]byte, hashAlgorithm.Size()),
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newNotransBlockProof(b *block.GenericBlock, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_NOTRANS,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          make([]byte, hashAlgorithm.Size()),
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newPrimBlockProof(b *block.GenericBlock, hashValue []byte, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_PRIM,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          hashValue,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newOnlysecBlockProof(b *block.GenericBlock, secHash []byte, chain []*omt.Data, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_ONLYSEC,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          secHash,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		UnicityCertificate: b.UnicityCertificate,
	}
}

func newSecBlockProof(b *block.GenericBlock, secHash []byte, chain []*omt.Data, secChain []*mt.PathItem, hashAlgorithm crypto.Hash) *BlockProofV2 {
	return &BlockProofV2{
		ProofType:          ProofType_SEC,
		BlockHeaderHash:    b.HashHeader(hashAlgorithm),
		HashValue:          secHash,
		BlockTreeHashChain: &BlockTreeHashChain{Items: ToProtobufHashChain(chain)},
		SecTreeHashChain:   &SecTreeHashChain{Items: ToProtobuf(secChain).PathItems},
		UnicityCertificate: b.UnicityCertificate,
	}
}
