package types

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/mt"
)

var (
	ErrBlockIsNil               = errors.New("block is nil")
	ErrBlockHeaderMissing       = errors.New("block header missing")
	ErrSystemIdentifierIsNil    = errors.New("system identifier is nil")
	ErrPrevBlockHeaderHashIsNil = errors.New("previous block header hash is nil")
	ErrBlockProposerIDIsMissing = errors.New("block proposer node identifier is missing")
	ErrTransactionsIsNil        = errors.New("transactions is nil")
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

func NewTxProof(block *Block, txIndex int, algorithm crypto.Hash) (*TxProof, error) {
	if block == nil {
		return nil, ErrBlockIsNil
	}
	if txIndex < 0 || txIndex > len(block.Transactions)-1 {
		return nil, fmt.Errorf("invalid tx index: %d", txIndex)
	}
	tree, err := mt.New(algorithm, block.Transactions)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate merkle tree root hash: %w", err)
	}
	headerHash := block.HeaderHash(algorithm)
	chain, err := tree.GetMerklePath(txIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to extract merkle proof: %w", err)
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
	}, nil
}

func VerifyTxProof(proof *TxProof, txRecord *TransactionRecord, trustBase map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	merklePath := make([]*mt.PathItem, len(proof.Chain))
	for i, item := range proof.Chain {
		merklePath[i] = &mt.PathItem{
			Hash:          item.Hash,
			DirectionLeft: item.Left,
		}
	}
	// h ← plain_tree_output(C, H(P))
	rootHash := mt.EvalMerklePath(merklePath, txRecord, hashAlgorithm)
	hasher := hashAlgorithm.New()
	hasher.Write(proof.BlockHeaderHash)
	hasher.Write(rootHash)
	//h ← H(h_h,h)
	blockHash := hasher.Sum(nil)

	// TODO ch 2.8.7: Verify Transaction Proof: VerifyTxProof: System description must be an input parameter
	systemDescriptionHash := proof.UnicityCertificate.UnicityTreeCertificate.SystemDescriptionHash
	if err := proof.UnicityCertificate.IsValid(trustBase, hashAlgorithm, txRecord.TransactionOrder.SystemID(), systemDescriptionHash); err != nil {
		return fmt.Errorf("invalid unicity certificate: %w", err)
	}
	//UC.IR.hB = h
	if !bytes.Equal(blockHash, proof.UnicityCertificate.InputRecord.BlockHash) {
		return fmt.Errorf("invalid chain root hash")
	}
	return nil
}
