package types

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/tree/mt"
)

var (
	ErrBlockIsNil = errors.New("block is nil")
)

type (
	// TxProof is a transaction execution proof.
	TxProof struct {
		_                  struct{} `cbor:",toarray"`
		BlockHeaderHash    []byte
		Chain              []*GenericChainItem
		UnicityCertificate *UnicityCertificate
	}

	GenericChainItem struct {
		_    struct{} `cbor:",toarray"`
		Hash []byte
		Left bool
	}
)

func (p *TxProof) GetUnicityTreeSystemDescriptionHash() []byte {
	if p == nil || p.UnicityCertificate == nil || p.UnicityCertificate.UnicityTreeCertificate == nil {
		return nil
	}
	return p.UnicityCertificate.UnicityTreeCertificate.SystemDescriptionHash
}

func NewTxProof(block *Block, txIndex int, algorithm crypto.Hash) (*TxProof, *TransactionRecord, error) {
	if block == nil {
		return nil, nil, ErrBlockIsNil
	}
	if txIndex < 0 || txIndex > len(block.Transactions)-1 {
		return nil, nil, fmt.Errorf("invalid tx index: %d", txIndex)
	}
	tree := mt.New(algorithm, block.Transactions)
	headerHash := block.HeaderHash(algorithm)
	chain, err := tree.GetMerklePath(txIndex)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract merkle proof: %w", err)
	}
	items := make([]*GenericChainItem, len(chain))
	for i, item := range chain {
		items[i] = &GenericChainItem{
			Left: item.DirectionLeft,
			Hash: item.Hash,
		}
	}
	return &TxProof{
		BlockHeaderHash:    headerHash,
		Chain:              items,
		UnicityCertificate: block.UnicityCertificate,
	}, block.Transactions[txIndex], nil
}

func VerifyTxProof(proof *TxProof, txRecord *TransactionRecord, trustBase map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	if proof == nil {
		return errors.New("tx proof is nil")
	}
	if txRecord == nil {
		return errors.New("tx record is nil")
	}
	if txRecord.TransactionOrder == nil {
		return errors.New("tx order is nil")
	}
	merklePath := make([]*mt.PathItem, len(proof.Chain))
	for i, item := range proof.Chain {
		merklePath[i] = &mt.PathItem{
			Hash:          item.Hash,
			DirectionLeft: item.Left,
		}
	}
	// TODO ch 2.8.7: Verify Transaction Proof: VerifyTxProof: System description must be an input parameter
	systemDescriptionHash := proof.GetUnicityTreeSystemDescriptionHash()
	if err := proof.UnicityCertificate.IsValid(trustBase, hashAlgorithm, txRecord.TransactionOrder.SystemID(), systemDescriptionHash); err != nil {
		return fmt.Errorf("invalid unicity certificate: %w", err)
	}
	// h ← plain_tree_output(C, H(P))
	rootHash := mt.EvalMerklePath(merklePath, txRecord, hashAlgorithm)
	hasher := hashAlgorithm.New()
	hasher.Write(proof.BlockHeaderHash)
	hasher.Write(proof.UnicityCertificate.InputRecord.PreviousHash)
	hasher.Write(proof.UnicityCertificate.InputRecord.Hash)
	hasher.Write(rootHash)
	//h ← H(h_h,h)
	blockHash := hasher.Sum(nil)

	//UC.IR.hB = h
	if !bytes.Equal(blockHash, proof.UnicityCertificate.InputRecord.BlockHash) {
		return fmt.Errorf("invalid chain root hash")
	}
	return nil
}
